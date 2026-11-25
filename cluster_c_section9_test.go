package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"
)

// 9.1: signaling spec with versioning — implementation vs spec.
func TestClusterCSignalingSpec(t *testing.T) {
	valid := []string{
		`{"version":1,"type":"join","room":"default","from":"viewer-1"}`,
		`{"version":1,"type":"offer","from":"pub-1","to":"viewer-1","payload":{"sdp":"v=0"}}`,
		`{"version":1,"type":"candidate","from":"pub-1","to":"viewer-1","payload":{"candidate":"candidate:1"}}`,
		`{"version":1,"type":"layer-switch","room":"default","from":"viewer-1","payload":{"layer":"top"}}`,
		`{"version":1,"type":"metadata","room":"default","from":"pub-1","payload":{"name":"x"}}`,
		`{"version":1,"type":"message","room":"default","from":"pub-1","payload":{"text":"hi"}}`,
		`{"version":1,"type":"resume","room":"default","from":"viewer-1"}`,
	}
	for _, raw := range valid {
		if _, err := parseSignalingMessage([]byte(raw)); err != nil {
			t.Fatalf("valid message rejected: %s: %v", raw, err)
		}
	}
	invalid := []struct {
		name string
		raw  string
	}{
		{"missing version", `{"type":"join","room":"default","from":"a"}`},
		{"version 0", `{"version":0,"type":"join","room":"default","from":"a"}`},
		{"future version 2", `{"version":2,"type":"join","room":"default","from":"a"}`},
		{"unknown type", `{"version":1,"type":"teleport","room":"default","from":"a"}`},
		{"join without room", `{"version":1,"type":"join","from":"a"}`},
		{"join without sender", `{"version":1,"type":"join","room":"default"}`},
		{"offer without payload", `{"version":1,"type":"offer","from":"a","to":"b"}`},
		{"offer without to", `{"version":1,"type":"offer","from":"a","payload":{}}`},
		{"not JSON", `not json`},
		{"unknown field", `{"version":1,"type":"join","room":"default","from":"a","surprise":1}`},
	}
	for _, c := range invalid {
		if _, err := parseSignalingMessage([]byte(c.raw)); err == nil {
			t.Fatalf("invalid message accepted (%s): %s", c.name, c.raw)
		}
	}
	if _, err := parseSignalingMessage([]byte(strings.Repeat("x", maxSignalingJSONBytes+1))); err == nil {
		t.Fatal("oversized signaling message accepted")
	}
	errMsg, err := signalingError("default", "viewer-1", "unauthorized", "bad token")
	if err != nil {
		t.Fatal(err)
	}
	if errMsg.Type != "error" || errMsg.Version != 1 {
		t.Fatalf("error message = %+v, want version 1 type error", errMsg)
	}
	t.Logf("9.1: spec v1 — 7 valid messages pass, 10 invalid forms rejected, oversize rejected")
}

// 9.2: room model — rooms hold participants, publish/subscribe tracks,
// each room has its own profile. Implemented behind the existing
// single-room behaviour; the live router migration is owed (see results doc).
func TestClusterCRoomModel(t *testing.T) {
	mgr := newRoomManager()
	cfg, _ := loadProfileConfig("")
	room, err := mgr.createRoom("room-1", "H264", cfg)
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	if room.Profile.byName()["top"].MaxKbps != 1200 {
		t.Fatal("room did not get its own profile")
	}
	// Duplicate room rejected; unknown codec rejected; empty name rejected.
	if _, err := mgr.createRoom("room-1", "H264", cfg); err == nil {
		t.Fatal("duplicate room accepted")
	}
	if _, err := mgr.createRoom("room-2", "VP8", cfg); err == nil {
		t.Fatal("room with unsupported codec accepted")
	}
	if _, err := mgr.createRoom("", "H264", cfg); err == nil {
		t.Fatal("room with empty name accepted")
	}
	// Participants: publisher + subscriber.
	if err := room.join(&roomParticipant{ID: "pub-1", Role: "publish"}); err != nil {
		t.Fatalf("publisher join: %v", err)
	}
	if err := room.join(&roomParticipant{ID: "viewer-1", Role: "subscribe"}); err != nil {
		t.Fatalf("viewer join: %v", err)
	}
	if err := room.join(&roomParticipant{ID: "viewer-1", Role: "subscribe"}); err == nil {
		t.Fatal("duplicate participant accepted")
	}
	// A subscriber may not publish.
	if err := room.publishTrack("viewer-1", "track-x", "H264", []string{"base"}); err == nil {
		t.Fatal("subscriber published a track")
	}
	// Publisher publishes a three-layer track; viewer subscribes.
	if err := room.publishTrack("pub-1", "track-1", "H264", []string{"top", "middle", "base"}); err != nil {
		t.Fatalf("publish track: %v", err)
	}
	if err := room.subscribe("viewer-1", "track-1"); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := room.subscribe("viewer-1", "track-missing"); err == nil {
		t.Fatal("subscribe to unpublished track accepted")
	}
	if got := room.subscriberCount("track-1"); got != 1 {
		t.Fatalf("subscribers = %d, want 1", got)
	}
	if got := room.participantCount(); got != 2 {
		t.Fatalf("participants = %d, want 2", got)
	}
	// Second room is independent: same participant ID, different room.
	room2, err := mgr.createRoom("room-2", "H264", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := room2.join(&roomParticipant{ID: "pub-1", Role: "publish"}); err != nil {
		t.Fatalf("same ID in a second room rejected: %v", err)
	}
	if got := len(mgr.listRooms()); got != 2 {
		t.Fatalf("rooms = %d, want 2", got)
	}
	if err := mgr.deleteRoom("room-2"); err != nil {
		t.Fatalf("delete room: %v", err)
	}
	if _, ok := mgr.getRoom("room-2"); ok {
		t.Fatal("deleted room still listed")
	}
	t.Logf("9.2 PARTIAL: room model implemented + tested behind existing single-room behaviour; live-router migration owed (does not touch main.go hot path in this session)")
}

// 9.4: admin API — create/list/delete/kick/mute/permission-change.
func TestClusterCAdminAPI(t *testing.T) {
	mgr := newRoomManager()
	webhooks := newWebhookDispatcher("http://127.0.0.1:9/nope", []byte("webhook-secret"))
	webhooks.sleep = func(time.Duration) {}
	srv := newAdminServer(mgr, []byte("admin-secret"), webhooks, newResumeStore())
	call := func(method, path, body string, auth bool) *httptest.ResponseRecorder {
		var rdr *strings.Reader
		if body == "" {
			rdr = strings.NewReader("")
		} else {
			rdr = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, rdr)
		if auth {
			req.Header.Set("Authorization", "Bearer admin-secret")
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}
	// No auth => 401 on every route.
	if rec := call(http.MethodGet, "/admin/rooms", "", false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated admin call = %d, want 401", rec.Code)
	}
	if rec := call(http.MethodGet, "/admin/rooms", "", true); rec.Code != http.StatusOK {
		t.Fatalf("list rooms = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	// Create.
	if rec := call(http.MethodPost, "/admin/rooms", `{"name":"room-1","codec":"H264"}`, true); rec.Code != http.StatusCreated {
		t.Fatalf("create room = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPost, "/admin/rooms", `{"name":"room-1","codec":"H264"}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("duplicate create = %d, want 400", rec.Code)
	}
	if rec := call(http.MethodPost, "/admin/rooms", `{"name":"room-9","codec":"VP8"}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("create with VP8 = %d, want 400", rec.Code)
	}
	// List shows room-1.
	if rec := call(http.MethodGet, "/admin/rooms", "", true); !strings.Contains(rec.Body.String(), "room-1") {
		t.Fatalf("list rooms missing room-1: %s", rec.Body.String())
	}
	// Join a participant directly, then kick / mute / permissions.
	room, _ := mgr.getRoom("room-1")
	if err := room.join(&roomParticipant{ID: "viewer-1", Role: "subscribe"}); err != nil {
		t.Fatal(err)
	}
	if rec := call(http.MethodPost, "/admin/rooms/room-1/participants/viewer-1/mute", `{"muted":true}`, true); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "true") {
		t.Fatalf("mute = %d %s, want 200 muted=true", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPost, "/admin/rooms/room-1/participants/viewer-1/permissions", `{"can_publish":true}`, true); rec.Code != http.StatusOK {
		t.Fatalf("permissions = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !room.participants["viewer-1"].CanPublish {
		t.Fatal("permission change did not grant publish")
	}
	if rec := call(http.MethodPost, "/admin/rooms/room-1/participants/viewer-1/kick", ``, true); rec.Code != http.StatusOK {
		t.Fatalf("kick = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if _, ok := room.participants["viewer-1"]; ok {
		t.Fatal("kicked participant still in room")
	}
	if rec := call(http.MethodPost, "/admin/rooms/room-1/participants/ghost/kick", ``, true); rec.Code != http.StatusNotFound {
		t.Fatalf("kick of missing participant = %d, want 404", rec.Code)
	}
	// Delete.
	if rec := call(http.MethodDelete, "/admin/rooms/room-1", "", true); rec.Code != http.StatusOK {
		t.Fatalf("delete room = %d, want 200", rec.Code)
	}
	if rec := call(http.MethodDelete, "/admin/rooms/room-1", "", true); rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing room = %d, want 404", rec.Code)
	}
	t.Logf("9.4: admin API create/list/delete/kick/mute/permission-change all tested; unauthenticated calls 401")
}

// 9.5: webhooks with retries + signatures.
func TestClusterCWebhooks(t *testing.T) {
	secret := []byte("webhook-secret")
	// The retry test below uses its own counting server for clarity.
	t.Run("retry-then-deliver", func(t *testing.T) {
		count := 0
		var gotSig string
		var gotBody []byte
		srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count++
			gotBody, _ = io.ReadAll(r.Body)
			gotSig = r.Header.Get("X-Hami-Signature")
			if count < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv2.Close()
		d := newWebhookDispatcher(srv2.URL, secret)
		d.sleep = func(time.Duration) {} // no real waiting in tests
		ev := webhookEvent{Type: "joined", Room: "room-1", ParticipantID: "viewer-1", At: time.Now()}
		if err := d.dispatch(ev); err != nil {
			t.Fatalf("webhook dispatch: %v", err)
		}
		if count != 3 {
			t.Fatalf("delivery attempts = %d, want 3 (2 failures + success)", count)
		}
		if !verifyWebhookSignature(secret, gotBody, gotSig) {
			t.Fatal("delivered webhook signature did not verify")
		}
		if got := d.deliveriesCopy(); len(got) != 1 || !got[0].OK || got[0].Attempts != 3 {
			t.Fatalf("delivery record = %+v, want 1 OK delivery with 3 attempts", got)
		}
	})
	t.Run("bad-signature-rejected", func(t *testing.T) {
		body := []byte(`{"type":"joined","room":"room-1"}`)
		if verifyWebhookSignature(secret, body, "00") {
			t.Fatal("bad signature accepted")
		}
		if verifyWebhookSignature(secret, body, signWebhook([]byte("wrong-secret"), body)) {
			t.Fatal("signature with the wrong secret accepted")
		}
		if !verifyWebhookSignature(secret, body, signWebhook(secret, body)) {
			t.Fatal("correct signature rejected")
		}
		if verifyWebhookSignature(secret, []byte(`{"type":"left"}`), signWebhook(secret, body)) {
			t.Fatal("signature for a different body accepted")
		}
	})
	t.Run("all-event-types", func(t *testing.T) {
		var seen []string
		srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var ev webhookEvent
			_ = json.Unmarshal(body, &ev)
			seen = append(seen, ev.Type)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv3.Close()
		d := newWebhookDispatcher(srv3.URL, secret)
		d.sleep = func(time.Duration) {}
		for _, typ := range []string{"joined", "left", "track-published", "room-ended"} {
			if err := d.dispatch(webhookEvent{Type: typ, Room: "room-1", At: time.Now()}); err != nil {
				t.Fatalf("dispatch %s: %v", typ, err)
			}
		}
		sort.Strings(seen)
		if fmt.Sprint(seen) != "[joined left room-ended track-published]" {
			t.Fatalf("events delivered = %v", seen)
		}
	})
	t.Logf("9.5: joined/left/track-published/room-ended deliver; failed deliveries retry (3 attempts); bad signatures rejected")
}

// 9.6: session resume after reconnect.
func TestClusterCSessionResume(t *testing.T) {
	store := newResumeStore()
	now := time.Now()
	store.drop(&resumeState{ParticipantID: "viewer-1", RoomName: "room-1", Role: "subscribe", Metadata: map[string]string{"name": "viewer"}, Tracks: nil})
	// Rejoin inside the 60 s window restores state.
	st, err := store.resume("viewer-1", now.Add(30*time.Second))
	if err != nil {
		t.Fatalf("resume inside window: %v", err)
	}
	if st.RoomName != "room-1" || st.Metadata["name"] != "viewer" {
		t.Fatalf("resumed state = %+v, want room-1 + metadata", st)
	}
	// Resume is single-use.
	if _, err := store.resume("viewer-1", now.Add(31*time.Second)); err == nil {
		t.Fatal("resume state was reusable after a successful resume")
	}
	// Rejoin after the window expires.
	store.drop(&resumeState{ParticipantID: "viewer-2", RoomName: "room-1", Role: "subscribe"})
	if _, err := store.resume("viewer-2", now.Add(61*time.Second)); err == nil {
		t.Fatal("resume after the 60s window was accepted")
	}
	if _, err := store.resume("ghost", now); err == nil {
		t.Fatal("resume with no stored state was accepted")
	}
	t.Logf("9.6: resume inside 60s window restores state; after 61s it expires; audio 1-min window is the precedent (main.go:3824)")
}

// 9.7: participant metadata propagation.
func TestClusterCMetadataPropagation(t *testing.T) {
	mgr := newRoomManager()
	cfg, _ := loadProfileConfig("")
	room, _ := mgr.createRoom("room-1", "H264", cfg)
	_ = room.join(&roomParticipant{ID: "pub-1", Role: "publish"})
	_ = room.join(&roomParticipant{ID: "viewer-1", Role: "subscribe"})
	_ = room.join(&roomParticipant{ID: "viewer-2", Role: "subscribe"})
	update, err := room.setMetadata("pub-1", map[string]string{"display_name": "Presenter", "hand_raised": "true"})
	if err != nil {
		t.Fatalf("set metadata: %v", err)
	}
	if update.Metadata["display_name"] != "Presenter" {
		t.Fatalf("metadata update = %+v", update)
	}
	// Fan-out audience is every participant in the room.
	audience := room.subscribersOf()
	sort.Strings(audience)
	if fmt.Sprint(audience) != "[pub-1 viewer-1 viewer-2]" {
		t.Fatalf("metadata audience = %v, want all 3 participants", audience)
	}
	if _, err := room.setMetadata("ghost", map[string]string{"x": "y"}); err == nil {
		t.Fatal("metadata set for a non-participant accepted")
	}
	t.Logf("9.7: metadata change reaches all 3 participants in the room")
}

// 9.8: data channel for messages, separate from media.
func TestClusterCDataChannelMessages(t *testing.T) {
	bus := newMessageBus()
	for _, id := range []string{"pub-1", "viewer-1", "viewer-2"} {
		bus.register("room-1", id)
	}
	n := bus.send(roomMessage{RoomID: "room-1", From: "pub-1", Text: "hello everyone", At: time.Now()})
	if n != 3 {
		t.Fatalf("message recipients = %d, want 3", n)
	}
	for _, id := range []string{"pub-1", "viewer-1", "viewer-2"} {
		inbox := bus.inbox(id)
		if len(inbox) != 1 || inbox[0].Text != "hello everyone" || inbox[0].From != "pub-1" {
			t.Fatalf("inbox of %s = %+v, want the message", id, inbox)
		}
	}
	// A message to another room does not leak into room-1.
	bus.register("room-2", "viewer-9")
	bus.send(roomMessage{RoomID: "room-2", From: "viewer-9", Text: "other room", At: time.Now()})
	if got := len(bus.inbox("viewer-1")); got != 1 {
		t.Fatalf("viewer-1 inbox grew to %d after a room-2 message", got)
	}
	t.Logf("9.8 PARTIAL: message reaches every participant in the room, in order, with no media involved; real Pion DataChannel wiring on the PeerConnection is owed (see results doc)")
}
