// Cluster D job 13.7: WHIP ingest.
//
// WHIP (WebRTC-HTTP Ingestion Protocol, RFC 9444) lets an encoder
// such as OBS publish into a room with a plain HTTP POST of an SDP
// offer; the server answers with an SDP answer. This handler accepts
// the offer, creates a PeerConnection with the room's codecs, and
// returns the answer. ffmpeg's WHIP muxer is used as the encoder in
// the evidence run (OBS is not installed in the VM); the Go test
// drives the same endpoint with a Pion-generated offer.
//
// RTMP ingest is NOT implemented: WHIP is the chosen ingest (one
// protocol, WebRTC-native, no separate RTMP server to operate). That
// choice is recorded in docs/cluster-d/13-ingest.md.

package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/pion/webrtc/v4"
)

// whipIngestHandler answers WHIP POST /whip/{room} requests.
type whipIngestHandler struct {
	api *webrtc.API
	// onTrack is called for each incoming track (tests assert it fires).
	onTrack func(roomID string, track *webrtc.TrackRemote)
	mu      sync.Mutex
	offers  int // WHIP offers accepted (handshake completed)
}

func (h *whipIngestHandler) offerCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.offers
}

func newWHIPIngestHandler(api *webrtc.API) *whipIngestHandler {
	return &whipIngestHandler{api: api}
}

func (h *whipIngestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	roomID := strings.TrimPrefix(r.URL.Path, "/whip/")
	roomID = strings.Trim(roomID, "/")
	if roomID == "" {
		http.Error(w, "room is required", http.StatusBadRequest)
		return
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/sdp") {
		http.Error(w, "Content-Type must be application/sdp", http.StatusUnsupportedMediaType)
		return
	}
	offerSDP, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read offer: "+err.Error(), http.StatusBadRequest)
		return
	}
	answer, err := h.handleOffer(roomID, string(offerSDP))
	if err != nil {
		http.Error(w, "WHIP offer rejected: "+err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", "/whip/"+roomID)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(answer))
}

func (h *whipIngestHandler) handleOffer(roomID, offerSDP string) (string, error) {
	if h.api == nil {
		return "", errors.New("WHIP: no WebRTC API configured")
	}
	pc, err := h.api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return "", fmt.Errorf("WHIP: new PeerConnection: %w", err)
	}
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if h.onTrack != nil {
			h.onTrack(roomID, track)
		}
	})
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP}); err != nil {
		_ = pc.Close()
		return "", fmt.Errorf("WHIP: invalid offer: %w", err)
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		return "", fmt.Errorf("WHIP: create answer: %w", err)
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		_ = pc.Close()
		return "", fmt.Errorf("WHIP: set local description: %w", err)
	}
	h.mu.Lock()
	h.offers++
	h.mu.Unlock()
	// The PeerConnection stays alive for the ingest session; the
	// caller (room) owns its lifetime. In this tree the connection
	// is tracked by the test; the server integration wires it to
	// the room's router when rooms land (9.2).
	return answer.SDP, nil
}
