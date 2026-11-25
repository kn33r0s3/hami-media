// Cluster C §9.2, §9.6, §9.7, §9.8 + §8.3 layer-switch gate.
//
// 9.2 Room model: rooms hold participants; participants publish and
// subscribe to tracks; each room has its own profile (6.1 ladder).
// This is implemented as a NEW room model BEHIND the existing
// single-room behaviour: the existing global router in main.go is
// untouched (the 117-test suite must not break), and RoomManager is
// the model the new signaling/admin/webhook paths use. What remains
// (recorded in docs/cluster-c-results.md §9.2): migrating the live
// router's publish/subscribe paths onto RoomManager — a refactor that
// touches main.go's hot path and is owed as a follow-up, not smuggled
// in here.
//
// 9.6 Session resume: a dropped client rejoins with its state inside
// the agreed window (60 s, matching audioReconnectWindow = 1 min in
// main.go:3824). Resume state is stored at drop time and expires
// after the window.
//
// 9.7 Participant metadata propagation: metadata changes are stored
// per participant and fanned out to every subscriber of the room.
//
// 9.8 Data channel for messages, separate from media: messageBus
// delivers a message to every participant in a room in-process. The
// real DataChannel wiring (Pion DataChannel on the PeerConnection)
// is owed — this bus is the ordering/delivery model it must match,
// and the test proves delivery to every participant.
package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// --- 8.3 layer-switch authorization ------------------------------------------

// authorizeLayerSwitch is the layer-switch path gate (§8.3): the token
// must grant "subscribe" to this identity AND the viewer must already
// be allowed on the track being switched. A subscribe-only token for
// another track, a publish-only token, or a forged token all fail.
func authorizeLayerSwitch(verifier *sfuAuthVerifier, token, identity, trackID string, allowedTracks map[string]bool) error {
	if verifier == nil {
		return errors.New("layer switch requires an auth verifier (fail closed)")
	}
	claims, err := verifier.verify(token, "subscribe")
	if err != nil {
		return fmt.Errorf("layer switch denied: %w", err)
	}
	if claims.Subject != identity {
		return errors.New("layer switch denied: token is not for this identity")
	}
	if !allowedTracks[trackID] {
		return fmt.Errorf("layer switch denied: viewer is not subscribed to track %q", trackID)
	}
	return nil
}

// --- 9.2 room model -----------------------------------------------------------

// roomParticipant is one participant in a room.
type roomParticipant struct {
	ID         string
	Role       string // "publish" | "subscribe"
	Metadata   map[string]string
	JoinedAt   time.Time
	Muted      bool
	CanPublish bool
}

// roomTrack is one published track in a room.
type roomTrack struct {
	TrackID     string
	PublisherID string
	Codec       string
	Layers      []string // simulcast layers present, e.g. ["top","middle","base"]
}

// room is one room: participants, tracks, and its own profile.
type room struct {
	mu           sync.RWMutex
	Name         string
	Profile      profileConfig
	participants map[string]*roomParticipant
	tracks       map[string]*roomTrack
	subscribers  map[string]map[string]bool // trackID -> participantID set
	createdAt    time.Time
}

// roomManager holds all rooms on a node (bounded by maxRoomsPerNode, §8.4).
type roomManager struct {
	mu    sync.Mutex
	rooms map[string]*room
}

func newRoomManager() *roomManager {
	return &roomManager{rooms: map[string]*room{}}
}

// createRoom creates a room with its own profile. The codec is
// resolved per room (§6.9).
func (m *roomManager) createRoom(name, codec string, cfg profileConfig) (*room, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("room name is required")
	}
	if _, err := codecForRoom(name, codec); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("room %q profile invalid: %w", name, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := validateRoomsPerNode(len(m.rooms)); err != nil {
		return nil, err
	}
	if _, exists := m.rooms[name]; exists {
		return nil, fmt.Errorf("room %q already exists", name)
	}
	r := &room{
		Name:         name,
		Profile:      cfg,
		participants: map[string]*roomParticipant{},
		tracks:       map[string]*roomTrack{},
		subscribers:  map[string]map[string]bool{},
		createdAt:    time.Now(),
	}
	m.rooms[name] = r
	return r, nil
}

// getRoom returns a room by name.
func (m *roomManager) getRoom(name string) (*room, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rooms[name]
	return r, ok
}

// listRooms returns room names in creation order (map order is not
// stable; callers sort if they need determinism).
func (m *roomManager) listRooms() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.rooms))
	for name := range m.rooms {
		out = append(out, name)
	}
	return out
}

// deleteRoom removes a room and all its participants/tracks.
func (m *roomManager) deleteRoom(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rooms[name]; !ok {
		return fmt.Errorf("room %q does not exist", name)
	}
	delete(m.rooms, name)
	return nil
}

// join adds a participant to a room.
func (r *room) join(p *roomParticipant) error {
	if p == nil || strings.TrimSpace(p.ID) == "" {
		return errors.New("participant needs an ID")
	}
	if p.Role != "publish" && p.Role != "subscribe" {
		return fmt.Errorf("unknown participant role %q", p.Role)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.participants[p.ID]; exists {
		return fmt.Errorf("participant %q already in room %q", p.ID, r.Name)
	}
	if p.Metadata == nil {
		p.Metadata = map[string]string{}
	}
	p.JoinedAt = time.Now()
	p.CanPublish = p.Role == "publish"
	r.participants[p.ID] = p
	return nil
}

// leave removes a participant and all its subscriptions.
func (r *room) leave(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.participants[id]; !ok {
		return fmt.Errorf("participant %q is not in room %q", id, r.Name)
	}
	delete(r.participants, id)
	for trackID, subs := range r.subscribers {
		delete(subs, id)
		if len(subs) == 0 {
			delete(r.subscribers, trackID)
		}
	}
	return nil
}

// setParticipantMuted updates a participant's mute flag under the room lock.
func (r *room) setParticipantMuted(id string, muted bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.participants[id]
	if !ok {
		return fmt.Errorf("participant %q is not in room %q", id, r.Name)
	}
	p.Muted = muted
	return nil
}

// setParticipantPermissions updates a participant's publish permission under the room lock.
func (r *room) setParticipantPermissions(id string, canPublish bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.participants[id]
	if !ok {
		return fmt.Errorf("participant %q is not in room %q", id, r.Name)
	}
	p.CanPublish = canPublish
	if canPublish {
		p.Role = "publish"
	}
	return nil
}

// publishTrack registers a track published by a participant.
func (r *room) publishTrack(publisherID, trackID, codec string, layers []string) error {
	if _, err := codecForRoom(r.Name, codec); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.participants[publisherID]
	if !ok {
		return fmt.Errorf("publisher %q is not in room %q", publisherID, r.Name)
	}
	if !p.CanPublish {
		return fmt.Errorf("participant %q may not publish in room %q", publisherID, r.Name)
	}
	n := 0
	for _, t := range r.tracks {
		if t.PublisherID == publisherID {
			n++
		}
	}
	if err := validateTracksPerParticipant(n + 1); err != nil {
		return err
	}
	if _, exists := r.tracks[trackID]; exists {
		return fmt.Errorf("track %q already published in room %q", trackID, r.Name)
	}
	r.tracks[trackID] = &roomTrack{TrackID: trackID, PublisherID: publisherID, Codec: codec, Layers: layers}
	return nil
}

// subscribe records a participant subscribing to a track.
func (r *room) subscribe(participantID, trackID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.participants[participantID]; !ok {
		return fmt.Errorf("participant %q is not in room %q", participantID, r.Name)
	}
	if _, ok := r.tracks[trackID]; !ok {
		return fmt.Errorf("track %q is not published in room %q", trackID, r.Name)
	}
	if r.subscribers[trackID] == nil {
		r.subscribers[trackID] = map[string]bool{}
	}
	r.subscribers[trackID][participantID] = true
	return nil
}

// subscriberCount returns how many participants subscribe to a track
// (dynacast input, §6.8).
func (r *room) subscriberCount(trackID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.subscribers[trackID])
}

// participantCount returns the number of participants in the room.
func (r *room) participantCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.participants)
}

// --- 9.7 metadata propagation -------------------------------------------------

// metadataUpdate is one metadata change fanned out to subscribers.
type metadataUpdate struct {
	RoomID        string
	ParticipantID string
	Metadata      map[string]string
	At            time.Time
}

// setMetadata updates a participant's metadata and returns the update
// to propagate to every subscriber of the room.
func (r *room) setMetadata(participantID string, metadata map[string]string) (metadataUpdate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.participants[participantID]
	if !ok {
		return metadataUpdate{}, fmt.Errorf("participant %q is not in room %q", participantID, r.Name)
	}
	merged := map[string]string{}
	for k, v := range p.Metadata {
		merged[k] = v
	}
	for k, v := range metadata {
		merged[k] = v
	}
	p.Metadata = merged
	return metadataUpdate{RoomID: r.Name, ParticipantID: participantID, Metadata: merged, At: time.Now()}, nil
}

// subscribersOf returns every participant ID subscribed to any track
// in the room (the metadata fan-out audience), plus publishers.
func (r *room) subscribersOf() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]bool{}
	for _, subs := range r.subscribers {
		for id := range subs {
			seen[id] = true
		}
	}
	for id := range r.participants {
		seen[id] = true
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

// --- 9.6 session resume -------------------------------------------------------

// sessionResumeWindow is the agreed window for a dropped client to
// rejoin with its state (60 s, matching audioReconnectWindow).
const sessionResumeWindow = time.Minute

// resumeState is the state kept for a dropped participant.
type resumeState struct {
	ParticipantID string
	RoomName      string
	Role          string
	Metadata      map[string]string
	Tracks        []string // tracks it published
	DroppedAt     time.Time
}

// resumeStore keeps dropped sessions until the window expires.
type resumeStore struct {
	mu     sync.Mutex
	states map[string]*resumeState // key: participantID
}

func newResumeStore() *resumeStore {
	return &resumeStore{states: map[string]*resumeState{}}
}

// drop records a participant's state at drop time.
func (s *resumeStore) drop(st *resumeState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st.DroppedAt = time.Now()
	s.states[st.ParticipantID] = st
}

// resume returns the stored state if the client rejoins inside the
// window; otherwise it reports expired (and deletes the state).
func (s *resumeStore) resume(participantID string, now time.Time) (*resumeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.states[participantID]
	if !ok {
		return nil, fmt.Errorf("no resume state for participant %q", participantID)
	}
	if now.Sub(st.DroppedAt) > sessionResumeWindow {
		delete(s.states, participantID)
		return nil, fmt.Errorf("resume window of %s expired for participant %q", sessionResumeWindow, participantID)
	}
	delete(s.states, participantID)
	return st, nil
}

// --- 9.8 data channel (message bus, separate from media) ---------------------

// roomMessage is one data-channel message.
type roomMessage struct {
	RoomID string
	From   string
	Text   string
	At     time.Time
}

// messageBus delivers messages to every participant in a room.
// It is the delivery model the Pion DataChannel wiring must match:
// one message in, one delivery per participant, in send order, with
// no media packets involved.
type messageBus struct {
	mu      sync.Mutex
	inboxes map[string][]roomMessage // participantID -> messages
	rooms   map[string][]string      // roomID -> participantIDs
}

func newMessageBus() *messageBus {
	return &messageBus{inboxes: map[string][]roomMessage{}, rooms: map[string][]string{}}
}

// register records a participant as a message recipient in a room.
func (b *messageBus) register(roomID, participantID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, id := range b.rooms[roomID] {
		if id == participantID {
			return
		}
	}
	b.rooms[roomID] = append(b.rooms[roomID], participantID)
}

// send delivers msg to every participant in the room, including the
// sender (echo, as a data channel does). Returns the recipient count.
func (b *messageBus) send(msg roomMessage) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	recipients := b.rooms[msg.RoomID]
	for _, id := range recipients {
		b.inboxes[id] = append(b.inboxes[id], msg)
	}
	return len(recipients)
}

// inbox returns a copy of a participant's received messages.
func (b *messageBus) inbox(participantID string) []roomMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]roomMessage(nil), b.inboxes[participantID]...)
}
