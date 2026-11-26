// Part 4: standalone browser-facing SFU server for streamers and viewers.
//
// -mode=serve starts the SFU on a user-specified TCP address (default
// 127.0.0.1:8080) with token auth from the environment (HAMI_SFU_HMAC_KEY
// or HAMI_SFU_HMAC_KEY_FILE). The browser sends its HMAC token (not the
// process bearer token) in the request body for /publisher and /viewer.
//
// This reuses the same mux, router, and handler code that runM7HSFUServer
// uses, except the listener is opened by this process instead of inherited
// from a parent harness.
//
// New routes added here:
//   POST /publisher/leave   – publisher stops without closing the room
//   GET  /rooms/{room}/members – live member list (JSON)
//   GET  /rooms/{room}/events  – Server-Sent Events (publisher-joined, publisher-left, chat)
//   POST /rooms/{room}/chat    – send a chat message (broadcast via SSE)
//   GET  /                     – serves static files from examples/ and sdk/web/

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
)

// sseEvent is one event pushed to connected SSE clients.
type sseEvent struct {
	Type string `json:"type"` // publisher-joined, publisher-left, chat
	Data any    `json:"data"`
}

// sseBroker fans out events to connected SSE clients for a room.
type sseBroker struct {
	mu      sync.RWMutex
	clients map[chan sseEvent]struct{}
}

func newSSEBroker() *sseBroker {
	return &sseBroker{clients: make(map[chan sseEvent]struct{})}
}

func (b *sseBroker) subscribe() chan sseEvent {
	ch := make(chan sseEvent, 64)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *sseBroker) unsubscribe(ch chan sseEvent) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

func (b *sseBroker) publish(ev sseEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- ev:
		default: // drop if client is slow
		}
	}
}

// serveState holds per-room state for the browser-facing server.
type serveState struct {
	mu          sync.Mutex
	members     map[string]*serveMember       // publisherID -> member
	brokers     map[string]*sseBroker         // room -> broker
	chat        map[string][]serveChatMessage // room -> last 50 messages
	chatLimiter *rateLimiter
}

type serveMember struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Room     string            `json:"room"`
	JoinedAt time.Time         `json:"joined_at"`
	Tracks   []serveMediaTrack `json:"tracks"`
}

type serveMediaTrack struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

func publisherMediaTracks(router *sfuRouter, publisherID string) []serveMediaTrack {
	router.mu.RLock()
	defer router.mu.RUnlock()

	byID := make(map[string]serveMediaTrack)
	for key, track := range router.tracks {
		if key.PublisherID != publisherID || track == nil {
			continue
		}
		if track.kind == "audio" || track.kind == "video" {
			byID[key.TrackID] = serveMediaTrack{ID: key.TrackID, Kind: track.kind}
		}
	}

	tracks := make([]serveMediaTrack, 0, len(byID))
	for _, track := range byID {
		tracks = append(tracks, track)
	}
	sort.Slice(tracks, func(i, j int) bool {
		if tracks[i].Kind != tracks[j].Kind {
			return tracks[i].Kind < tracks[j].Kind
		}
		return tracks[i].ID < tracks[j].ID
	})
	return tracks
}

type serveChatMessage struct {
	From string    `json:"from"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

func newServeState() *serveState {
	return &serveState{
		members:     make(map[string]*serveMember),
		brokers:     make(map[string]*sseBroker),
		chat:        make(map[string][]serveChatMessage),
		chatLimiter: newRateLimiter(joinLimitPerIPPerMin, time.Minute),
	}
}

func (s *serveState) broker(room string) *sseBroker {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.brokers[room]
	if !ok {
		b = newSSEBroker()
		s.brokers[room] = b
	}
	return b
}

func (s *serveState) addMember(m *serveMember) {
	s.mu.Lock()
	s.members[m.ID] = m
	s.mu.Unlock()
	s.broker(m.Room).publish(sseEvent{Type: "publisher-joined", Data: m})
}

func (s *serveState) removeMember(id string) {
	s.mu.Lock()
	m, ok := s.members[id]
	if ok {
		delete(s.members, id)
	}
	s.mu.Unlock()
	if ok {
		s.broker(m.Room).publish(sseEvent{Type: "publisher-left", Data: map[string]string{"id": id}})
	}
}

func (s *serveState) listMembers(room string) []*serveMember {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*serveMember
	for _, m := range s.members {
		if m.Room == room {
			out = append(out, m)
		}
	}
	return out
}

func (s *serveState) addChat(room string, msg serveChatMessage) {
	s.mu.Lock()
	msgs := append(s.chat[room], msg)
	if len(msgs) > 50 {
		msgs = msgs[len(msgs)-50:]
	}
	s.chat[room] = msgs
	s.mu.Unlock()
	s.broker(room).publish(sseEvent{Type: "chat", Data: msg})
}

func (s *serveState) recentChat(room string) []serveChatMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]serveChatMessage(nil), s.chat[room]...)
}

// runServeMode starts the browser-facing SFU on listenAddr.
func runServeMode(listenAddr string) error {
	authVerifier, err := loadSFUAuthVerifier()
	if err != nil {
		return fmt.Errorf("load token auth: %w", err)
	}

	shardCount := 1
	server := &m7hRemoteSFUServer{
		auth:            authVerifier,
		requireAuth:     true,
		apis:            make([]*webrtc.API, shardCount),
		muxes:           make([]ice.UDPMux, 0, shardCount),
		muxByShard:      make([]ice.UDPMux, shardCount),
		settingsByShard: make([]webrtc.SettingEngine, shardCount),
		publishers:      make(map[string]m7hRemotePublisherState, 64),
		viewers:         make(map[string]viewerEndpoint, 64),
		maxPublishers:   100,
		maxSubscribers:  500,
	}

	settings, mux, settingsErr := loopbackSettings(false, 0)
	if settingsErr != nil {
		return fmt.Errorf("configure shard 0: %w", settingsErr)
	}
	server.settingsByShard[0] = settings
	server.muxByShard[0] = mux
	server.apis[0] = webrtc.NewAPI(webrtc.WithSettingEngine(settings))
	if mux != nil {
		server.muxes = append(server.muxes, mux)
	}

	server.router, err = newSFURouter(server.apis[0])
	if err != nil {
		return fmt.Errorf("create router: %w", err)
	}
	server.router.m7hPublisherReceiveStats = make(map[string]*m7hPublisherReceiveStats, 64)
	defer func() {
		if server.router != nil {
			_ = server.router.close()
		}
		for _, m := range server.muxes {
			_ = m.Close()
		}
	}()

	state := newServeState()

	httpMux := newBrowserServeMux(server, state, authVerifier)

	// Add CORS middleware
	handler := corsMiddleware(httpMux)

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", listenAddr, err)
	}

	httpServer := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "serve: shutting down...\n")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()

	fmt.Fprintf(os.Stderr, "serve: listening on %s, room=%s, auth=on\n", listener.Addr().String(), authVerifier.room)
	if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// newBrowserServeMux extends the one shared SFU route table with browser-only
// endpoints and overrides authentication/signaling where the browser contract differs.
func newBrowserServeMux(server *m7hRemoteSFUServer, state *serveState, authVerifier *sfuAuthVerifier) *http.ServeMux {
	return newM7HSFUMux(server, "", m7hSFURouteOverrides{
		handlers: map[string]http.Handler{
			"/": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, `<html><body><h1>Hami Media Server</h1><ul><li><a href="/examples/streamer.html">Streamer</a></li><li><a href="/examples/viewer.html">Viewer</a></li><li><a href="/health">Health</a></li><li><a href="/stats">Stats</a></li></ul></body></html>`)
			}),
			"/health": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeM7HJSON(w, http.StatusOK, map[string]string{"status": "ready"})
			}),
			"/publisher":       serveHandlePublisher(server, state),
			"/publisher/leave": serveHandlePublisherLeave(server, state),
			"/viewer":          serveHandleViewer(server, state),
			"/stats": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				server.mu.RLock()
				pubs, viewers := len(server.publishers), len(server.viewers)
				server.mu.RUnlock()
				writeM7HJSON(w, http.StatusOK, map[string]any{"publishers": pubs, "viewers": viewers})
			}),
		},
		metrics: server.router.metricsHandler(),
		register: func(mux *http.ServeMux) {
			mux.Handle("/publisher/leave", serveHandlePublisherLeave(server, state))
			mux.HandleFunc("/rooms/", func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, "/rooms/")
				parts := strings.SplitN(path, "/", 2)
				if len(parts) < 2 {
					writeM7HJSONError(w, http.StatusNotFound, fmt.Errorf("unknown route"))
					return
				}
				room, action := parts[0], parts[1]
				switch {
				case action == "members" && r.Method == http.MethodGet:
					token := r.URL.Query().Get("token")
					if _, err := authVerifier.verify(token, sfuTokenRoleSubscribe); err != nil {
						writeM7HJSONError(w, http.StatusUnauthorized, fmt.Errorf("invalid or expired token"))
						return
					}
					present := state.listMembers(room)
					members := make([]*serveMember, 0, len(present))
					for _, member := range present {
						withTracks := *member
						withTracks.Tracks = publisherMediaTracks(server.router, member.ID)
						members = append(members, &withTracks)
					}
					writeM7HJSON(w, http.StatusOK, map[string]any{"members": members})
				case action == "events" && r.Method == http.MethodGet:
					token := r.URL.Query().Get("token")
					if _, err := authVerifier.verify(token, sfuTokenRoleSubscribe); err != nil {
						writeM7HJSONError(w, http.StatusUnauthorized, fmt.Errorf("invalid or expired token"))
						return
					}
					serveSSE(w, r, state, room)
				case action == "chat" && r.Method == http.MethodPost:
					serveHandleChat(w, r, authVerifier, state, room)
				default:
					writeM7HJSONError(w, http.StatusNotFound, fmt.Errorf("unknown route"))
				}
			})
			examplesDir, sdkDir := "examples", "sdk/web"
			mux.HandleFunc("/examples/", func(w http.ResponseWriter, r *http.Request) {
				http.StripPrefix("/examples/", http.FileServer(http.Dir(examplesDir))).ServeHTTP(w, r)
			})
			mux.HandleFunc("/sdk/web/", func(w http.ResponseWriter, r *http.Request) {
				http.StripPrefix("/sdk/web/", http.FileServer(http.Dir(sdkDir))).ServeHTTP(w, r)
			})
		},
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// serveHandlePublisher handles POST /publisher for browser clients.
// Auth is via HMAC token in the JSON body (not bearer header).
func serveHandlePublisher(server *m7hRemoteSFUServer, state *serveState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeM7HJSONError(w, http.StatusMethodNotAllowed, fmt.Errorf("POST required"))
			return
		}
		var request struct {
			PublisherID string `json:"publisher_id"`
			Offer       string `json:"offer"`
			Token       string `json:"token"`
			DisplayName string `json:"display_name"`
		}
		if err := decodeM7HJSON(w, r, &request); err != nil {
			writeM7HJSONError(w, http.StatusBadRequest, err)
			return
		}
		if request.PublisherID == "" || request.Offer == "" {
			writeM7HJSONError(w, http.StatusBadRequest, fmt.Errorf("publisher_id and offer are required"))
			return
		}
		if !server.authorizeToken(w, request.Token, sfuTokenRolePublish, request.PublisherID) {
			return
		}
		server.mu.Lock()
		if _, exists := server.publishers[request.PublisherID]; exists {
			server.mu.Unlock()
			writeM7HJSONError(w, http.StatusConflict, fmt.Errorf("publisher %q already exists", request.PublisherID))
			return
		}
		if server.publisherLimitReached() {
			server.mu.Unlock()
			writeM7HJSONError(w, http.StatusServiceUnavailable, fmt.Errorf("publisher limit reached"))
			return
		}
		shard := server.nextPublisherShard % len(server.apis)
		server.nextPublisherShard++
		server.publishers[request.PublisherID] = m7hRemotePublisherState{shard: shard}
		receiveStats := &m7hPublisherReceiveStats{publisherID: request.PublisherID}
		server.mu.Unlock()
		server.router.mu.Lock()
		server.router.m7hPublisherReceiveStats[request.PublisherID] = receiveStats
		server.router.mu.Unlock()
		peer, err := server.router.registerPublisher(server.apis[shard], request.PublisherID)
		if err != nil {
			server.discardFailedPublisher(request.PublisherID)
			writeM7HJSONError(w, http.StatusBadRequest, err)
			return
		}
		server.mu.Lock()
		ps := server.publishers[request.PublisherID]
		ps.peer = peer
		server.publishers[request.PublisherID] = ps
		server.mu.Unlock()
		answer, err := answerM7HRemoteOffer(r.Context(), peer, request.Offer)
		if err != nil {
			server.discardFailedPublisher(request.PublisherID)
			writeM7HJSONError(w, http.StatusBadRequest, err)
			return
		}
		// Add member to presence list.
		name := request.DisplayName
		if name == "" {
			name = request.PublisherID
		}
		claims, _ := server.auth.verify(request.Token, sfuTokenRolePublish)
		room := ""
		if claims != nil {
			room = claims.Room
		}
		state.addMember(&serveMember{
			ID:       request.PublisherID,
			Name:     name,
			Room:     room,
			JoinedAt: time.Now(),
			Tracks:   []serveMediaTrack{},
		})
		writeM7HJSON(w, http.StatusOK, m7hRemoteSDPResponse{Answer: answer})
	}
}

// serveHandlePublisherLeave handles POST /publisher/leave for browser clients (S7).
func serveHandlePublisherLeave(server *m7hRemoteSFUServer, state *serveState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeM7HJSONError(w, http.StatusMethodNotAllowed, fmt.Errorf("POST required"))
			return
		}
		var request struct {
			PublisherID string `json:"publisher_id"`
			Token       string `json:"token"`
		}
		if err := decodeM7HJSON(w, r, &request); err != nil {
			writeM7HJSONError(w, http.StatusBadRequest, err)
			return
		}
		if request.PublisherID == "" {
			writeM7HJSONError(w, http.StatusBadRequest, fmt.Errorf("publisher_id is required"))
			return
		}
		if !server.authorizeToken(w, request.Token, sfuTokenRolePublish, request.PublisherID) {
			return
		}
		// Remove publisher tracks from all viewers.
		for _, key := range server.router.trackKeysForPublisher(request.PublisherID) {
			_ = server.router.unregisterTrack(key)
		}
		_ = server.router.unregisterPublisher(request.PublisherID)
		server.mu.Lock()
		pstate, exists := server.publishers[request.PublisherID]
		if exists {
			if pstate.peer != nil {
				_ = pstate.peer.Close()
			}
			delete(server.publishers, request.PublisherID)
		}
		server.router.mu.Lock()
		delete(server.router.m7hPublisherReceiveStats, request.PublisherID)
		server.router.mu.Unlock()
		server.mu.Unlock()
		state.removeMember(request.PublisherID)
		writeM7HJSON(w, http.StatusOK, map[string]string{"left": request.PublisherID})
	}
}

// serveHandleViewer handles POST /viewer for browser clients.
func serveHandleViewer(server *m7hRemoteSFUServer, state *serveState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeM7HJSONError(w, http.StatusMethodNotAllowed, fmt.Errorf("POST required"))
			return
		}
		var request m7hRemoteViewerRequest
		if err := decodeM7HJSON(w, r, &request); err != nil {
			writeM7HJSONError(w, http.StatusBadRequest, err)
			return
		}
		subscriberID := request.subscriberID()
		if strings.TrimSpace(request.ViewerID) == "" || subscriberID == "" || request.Key.PublisherID == "" ||
			request.Key.TrackID == "" || request.Offer == "" {
			writeM7HJSONError(w, http.StatusBadRequest, fmt.Errorf("viewer_id, key (publisher_id, track_id), and offer are required"))
			return
		}
		if !server.authorizeToken(w, request.Token, sfuTokenRoleSubscribe, request.ViewerID) {
			return
		}
		if err := server.router.waitForTracks(r.Context(), []sfuMediaSource{{key: request.Key}}); err != nil {
			writeM7HJSONError(w, http.StatusRequestTimeout, err)
			return
		}
		server.mu.RLock()
		publisher, exists := server.publishers[request.Key.PublisherID]
		server.mu.RUnlock()
		if !exists {
			writeM7HJSONError(w, http.StatusNotFound, fmt.Errorf("publisher %s is not registered", request.Key.PublisherID))
			return
		}
		routerPeerAPI := server.apis[publisher.shard]
		serverPeer, err := routerPeerAPI.NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			writeM7HJSONError(w, http.StatusInternalServerError, err)
			return
		}
		if err := server.router.registerViewer(subscriberID, serverPeer); err != nil {
			_ = serverPeer.Close()
			writeM7HJSONError(w, http.StatusConflict, err)
			return
		}
		if err := server.router.authorizeSubscription(subscriberID, request.Key); err != nil {
			_ = server.router.removeViewer(subscriberID)
			writeM7HJSONError(w, http.StatusInternalServerError, err)
			return
		}
		if err := server.router.subscribe(subscriberID, request.Key); err != nil {
			_ = server.router.removeViewer(subscriberID)
			writeM7HJSONError(w, http.StatusInternalServerError, err)
			return
		}
		answer, err := answerM7HRemoteOffer(r.Context(), serverPeer, request.Offer)
		if err != nil {
			_ = server.router.unsubscribe(subscriberID, request.Key)
			_ = server.router.removeViewer(subscriberID)
			writeM7HJSONError(w, http.StatusBadRequest, err)
			return
		}
		server.mu.Lock()
		if _, exists := server.viewers[subscriberID]; exists {
			server.mu.Unlock()
			_ = server.router.unsubscribe(subscriberID, request.Key)
			_ = server.router.removeViewer(subscriberID)
			writeM7HJSONError(w, http.StatusConflict, fmt.Errorf("viewer %q already exists", subscriberID))
			return
		}
		if server.subscriberLimitReached() {
			server.mu.Unlock()
			_ = server.router.unsubscribe(subscriberID, request.Key)
			_ = server.router.removeViewer(subscriberID)
			writeM7HJSONError(w, http.StatusServiceUnavailable, fmt.Errorf("subscriber limit reached"))
			return
		}
		server.viewers[subscriberID] = viewerEndpoint{
			id:  subscriberID,
			key: request.Key,
		}
		server.mu.Unlock()
		writeM7HJSON(w, http.StatusOK, m7hRemoteSDPResponse{Answer: answer})
	}
}

// serveSSE streams Server-Sent Events for a room.
func serveSSE(w http.ResponseWriter, r *http.Request, state *serveState, room string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeM7HJSONError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Send recent chat history as initial events.
	for _, msg := range state.recentChat(room) {
		data, _ := json.Marshal(msg)
		fmt.Fprintf(w, "event: chat\ndata: %s\n\n", data)
	}
	flusher.Flush()

	ch := state.broker(room).subscribe()
	defer state.broker(room).unsubscribe(ch)

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(ev.Data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// serveHandleChat handles POST /rooms/{room}/chat.
func serveHandleChat(w http.ResponseWriter, r *http.Request, auth *sfuAuthVerifier, state *serveState, room string) {
	var request struct {
		Token string `json:"token"`
		Text  string `json:"text"`
		From  string `json:"from"`
	}
	if err := decodeM7HJSON(w, r, &request); err != nil {
		writeM7HJSONError(w, http.StatusBadRequest, err)
		return
	}

	// Either publish or subscribe token is acceptable for chat.
	_, pubErr := auth.verify(request.Token, sfuTokenRolePublish)
	_, subErr := auth.verify(request.Token, sfuTokenRoleSubscribe)
	if pubErr != nil && subErr != nil {
		writeM7HJSONError(w, http.StatusUnauthorized, fmt.Errorf("invalid or expired token"))
		return
	}

	if len(request.Text) > 300 {
		writeM7HJSONError(w, http.StatusBadRequest, fmt.Errorf("text exceeds 300 characters"))
		return
	}
	if strings.TrimSpace(request.Text) == "" {
		writeM7HJSONError(w, http.StatusBadRequest, fmt.Errorf("text is required"))
		return
	}

	// Rate limit by remote IP.
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx >= 0 {
		ip = ip[:idx]
	}
	if !state.chatLimiter.allow(ip, time.Now()) {
		writeM7HJSONError(w, http.StatusTooManyRequests, fmt.Errorf("rate limit exceeded"))
		return
	}

	from := request.From
	if from == "" {
		from = "anonymous"
	}

	msg := serveChatMessage{
		From: from,
		Text: request.Text,
		At:   time.Now(),
	}
	state.addChat(room, msg)
	writeM7HJSON(w, http.StatusOK, map[string]string{"sent": "ok"})
}

// viewerEndpointServe is a minimal version of viewerEndpoint for serve mode.
// (The full viewerEndpoint is in main.go; this comment clarifies the serve
// mode reuses it.)

// findExamplesDir returns the directory for examples files, relative to the binary.
func findExamplesDir() string {
	ex, err := os.Executable()
	if err != nil {
		return "examples"
	}
	dir := filepath.Dir(ex)
	if _, err := os.Stat(filepath.Join(dir, "examples")); err == nil {
		return filepath.Join(dir, "examples")
	}
	return "examples"
}
