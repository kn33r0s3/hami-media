// Cluster C §9.4 admin API + §9.5 webhooks.
//
// 9.4 Admin API: create/list/delete rooms, kick, mute, permission
// change. Each operation is an HTTP handler on adminServer with a
// documented JSON schema (docs/cluster-c-admin-api.md) and a test in
// cluster_c_section9_test.go. Admin calls require an admin token:
// the same HMAC verifier with role "admin" is NOT in the token roles
// (publish/subscribe only), so admin auth is a separate bearer secret
// checked by requireSecret-style comparison — see adminAuth.
//
// 9.5 Webhooks: joined/left/track-published/room-ended events deliver
// to a configured URL with HMAC-SHA256 signatures; failed deliveries
// retry with backoff; a receiver verifies the signature and rejects
// bad ones (verifyWebhookSignature). The dispatcher is in-process and
// testable with httptest servers; the retry schedule is stated:
// 3 attempts, 1s / 2s / 4s backoff.
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// --- 9.4 admin API ------------------------------------------------------------

// adminServer serves the admin API over a roomManager.
type adminServer struct {
	rooms    *roomManager
	adminKey []byte // bearer secret, compared in constant time
	webhooks *webhookDispatcher
	resumes  *resumeStore
}

func newAdminServer(rooms *roomManager, adminKey []byte, webhooks *webhookDispatcher, resumes *resumeStore) *adminServer {
	return &adminServer{rooms: rooms, adminKey: adminKey, webhooks: webhooks, resumes: resumes}
}

// adminAuth checks the Authorization: Bearer <key> header in constant
// time. Missing/wrong => 401. The key never appears in a response.
func (s *adminServer) adminAuth(r *http.Request) bool {
	if len(s.adminKey) == 0 {
		return false
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if got == "" {
		return false
	}
	return hmac.Equal([]byte(got), s.adminKey)
}

func (s *adminServer) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !s.adminAuth(r) {
		writeM7HJSONError(w, http.StatusUnauthorized, errors.New("admin authorization required"))
		return false
	}
	return true
}

// ServeHTTP routes the admin API. Schema: docs/cluster-c-admin-api.md.
func (s *adminServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/admin/rooms":
		s.handleCreateRoom(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/admin/rooms":
		s.handleListRooms(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/admin/rooms/"):
		s.handleDeleteRoom(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/kick"):
		s.handleKick(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/mute"):
		s.handleMute(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/permissions"):
		s.handlePermissions(w, r)
	default:
		writeM7HJSONError(w, http.StatusNotFound, errors.New("unknown admin route"))
	}
}

func decodeAdminJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSignalingJSONBytes))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

func writeAdminJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *adminServer) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Codec string `json:"codec"`
	}
	if err := decodeAdminJSON(r, &req); err != nil {
		writeM7HJSONError(w, http.StatusBadRequest, err)
		return
	}
	cfg, _ := loadProfileConfig("")
	room, err := s.rooms.createRoom(req.Name, req.Codec, cfg)
	if err != nil {
		writeM7HJSONError(w, http.StatusBadRequest, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, map[string]any{"room": room.Name, "participants": 0})
}

func (s *adminServer) handleListRooms(w http.ResponseWriter, r *http.Request) {
	names := s.rooms.listRooms()
	sort.Strings(names)
	writeAdminJSON(w, http.StatusOK, map[string]any{"rooms": names})
}

func (s *adminServer) handleDeleteRoom(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/admin/rooms/")
	if name == "" || strings.Contains(name, "/") {
		writeM7HJSONError(w, http.StatusNotFound, errors.New("unknown admin route"))
		return
	}
	if err := s.rooms.deleteRoom(name); err != nil {
		writeM7HJSONError(w, http.StatusNotFound, err)
		return
	}
	if s.webhooks != nil {
		_ = s.webhooks.dispatch(webhookEvent{Type: "room-ended", Room: name, At: time.Now()})
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{"deleted": name})
}

func roomAndParticipant(path string) (roomName, participantID, action string, ok bool) {
	// /admin/rooms/<room>/participants/<id>/<kick|mute|permissions>
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 6 || parts[0] != "admin" || parts[1] != "rooms" || parts[3] != "participants" {
		return "", "", "", false
	}
	return parts[2], parts[4], parts[5], true
}

func (s *adminServer) handleKick(w http.ResponseWriter, r *http.Request) {
	roomName, participantID, _, ok := roomAndParticipant(r.URL.Path)
	if !ok {
		writeM7HJSONError(w, http.StatusNotFound, errors.New("unknown admin route"))
		return
	}
	room, exists := s.rooms.getRoom(roomName)
	if !exists {
		writeM7HJSONError(w, http.StatusNotFound, fmt.Errorf("room %q does not exist", roomName))
		return
	}
	if err := room.leave(participantID); err != nil {
		writeM7HJSONError(w, http.StatusNotFound, err)
		return
	}
	if s.webhooks != nil {
		_ = s.webhooks.dispatch(webhookEvent{Type: "left", Room: roomName, ParticipantID: participantID, At: time.Now()})
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{"kicked": participantID, "room": roomName})
}

func (s *adminServer) handleMute(w http.ResponseWriter, r *http.Request) {
	roomName, participantID, _, ok := roomAndParticipant(r.URL.Path)
	if !ok {
		writeM7HJSONError(w, http.StatusNotFound, errors.New("unknown admin route"))
		return
	}
	var req struct {
		Muted bool `json:"muted"`
	}
	if err := decodeAdminJSON(r, &req); err != nil {
		writeM7HJSONError(w, http.StatusBadRequest, err)
		return
	}
	room, exists := s.rooms.getRoom(roomName)
	if !exists {
		writeM7HJSONError(w, http.StatusNotFound, fmt.Errorf("room %q does not exist", roomName))
		return
	}
	if err := room.setParticipantMuted(participantID, req.Muted); err != nil {
		writeM7HJSONError(w, http.StatusNotFound, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{"participant": participantID, "muted": req.Muted})
}

func (s *adminServer) handlePermissions(w http.ResponseWriter, r *http.Request) {
	roomName, participantID, _, ok := roomAndParticipant(r.URL.Path)
	if !ok {
		writeM7HJSONError(w, http.StatusNotFound, errors.New("unknown admin route"))
		return
	}
	var req struct {
		CanPublish bool `json:"can_publish"`
	}
	if err := decodeAdminJSON(r, &req); err != nil {
		writeM7HJSONError(w, http.StatusBadRequest, err)
		return
	}
	room, exists := s.rooms.getRoom(roomName)
	if !exists {
		writeM7HJSONError(w, http.StatusNotFound, fmt.Errorf("room %q does not exist", roomName))
		return
	}
	if err := room.setParticipantPermissions(participantID, req.CanPublish); err != nil {
		writeM7HJSONError(w, http.StatusNotFound, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{"participant": participantID, "can_publish": req.CanPublish})
}

// --- 9.5 webhooks -------------------------------------------------------------

// webhookEvent is one event delivered to subscribers.
type webhookEvent struct {
	Type          string    `json:"type"` // joined|left|track-published|room-ended
	Room          string    `json:"room"`
	ParticipantID string    `json:"participant_id,omitempty"`
	TrackID       string    `json:"track_id,omitempty"`
	At            time.Time `json:"at"`
}

// webhookRetryDelays is the stated retry schedule: 3 attempts.
var webhookRetryDelays = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

// webhookDispatcher delivers events to one URL with HMAC signatures.
type webhookDispatcher struct {
	mu         sync.Mutex
	url        string
	secret     []byte
	client     *http.Client
	deliveries []webhookDelivery
	// sleep is injectable so tests run retries without waiting.
	sleep func(time.Duration)
}

type webhookDelivery struct {
	Event    webhookEvent
	Attempts int
	OK       bool
}

func newWebhookDispatcher(url string, secret []byte) *webhookDispatcher {
	return &webhookDispatcher{
		url:    url,
		secret: secret,
		client: &http.Client{Timeout: 5 * time.Second},
		sleep:  time.Sleep,
	}
}

// signWebhook computes HMAC-SHA256(secret, body) hex.
func signWebhook(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyWebhookSignature checks a receiver-side signature in constant
// time. A bad signature is rejected.
func verifyWebhookSignature(secret []byte, body []byte, signature string) bool {
	want := signWebhook(secret, body)
	got, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	wantBytes, err := hex.DecodeString(want)
	if err != nil {
		return false
	}
	return hmac.Equal(got, wantBytes)
}

// dispatch delivers one event, retrying failed deliveries per the
// stated schedule. It returns the final error (nil on success).
func (d *webhookDispatcher) dispatch(event webhookEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	var lastErr error
	attempts := 0
	for i := 0; i <= len(webhookRetryDelays); i++ {
		attempts++
		req, err := http.NewRequest(http.MethodPost, d.url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Hami-Signature", signWebhook(d.secret, body))
		req.Header.Set("X-Hami-Event", event.Type)
		resp, err := d.client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			_ = resp.Body.Close()
			d.record(event, attempts, true)
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("webhook delivery to %s returned %d", d.url, resp.StatusCode)
		} else {
			lastErr = err
		}
		if i < len(webhookRetryDelays) {
			d.sleep(webhookRetryDelays[i])
		}
	}
	d.record(event, attempts, false)
	return lastErr
}

func (d *webhookDispatcher) record(event webhookEvent, attempts int, ok bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deliveries = append(d.deliveries, webhookDelivery{Event: event, Attempts: attempts, OK: ok})
}

func (d *webhookDispatcher) deliveriesCopy() []webhookDelivery {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]webhookDelivery(nil), d.deliveries...)
}
