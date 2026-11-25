package main

import (
	"testing"
	"time"
)

func TestClusterDRegistryDrainClosesNoRooms(t *testing.T) {
	reg := newClusterDNodeRegistry()
	if err := reg.register("node-a", "10.0.0.1:9000", 10, 1000); err != nil {
		t.Fatal(err)
	}
	// Place 3 rooms while node-a is the only node: all land on it.
	for _, room := range []string{"room-1", "room-2", "room-3"} {
		if _, err := reg.placeRoom(room); err != nil {
			t.Fatalf("place %s: %v", room, err)
		}
	}
	if got := reg.roomsOf("node-a"); len(got) != 3 {
		t.Fatalf("node-a should hold 3 rooms, holds %v", got)
	}
	if err := reg.register("node-b", "10.0.0.2:9000", 10, 1000); err != nil {
		t.Fatal(err)
	}
	// Drain node-a: it must receive no new rooms (11.1 Done line:
	// a drained node closes no rooms in the test — i.e. no new room
	// is placed on it, and its existing rooms are untouched here).
	if err := reg.drain("node-a"); err != nil {
		t.Fatal(err)
	}
	node, err := reg.placeRoom("room-4")
	if err != nil {
		t.Fatalf("place room-4: %v", err)
	}
	if node == "node-a" {
		t.Fatal("drained node-a received a new room")
	}
	if got := reg.roomsOf("node-a"); len(got) != 3 {
		t.Fatalf("drain changed node-a rooms: %v", got)
	}
	if got := reg.roomsOf("node-b"); len(got) != 1 || got[0] != "room-4" {
		t.Fatalf("room-4 should be on node-b, got %v", got)
	}
	if states := reg.health(); states["node-a"] != clusterDNodeDrained {
		t.Fatalf("node-a health %v, want drained", states["node-a"])
	}
}

func TestClusterDPlacementSpreadsUnderLoad(t *testing.T) {
	reg := newClusterDNodeRegistry()
	for _, id := range []string{"n1", "n2", "n3"} {
		if err := reg.register(id, "10.0.0.1:9000", 10, 1000); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 30; i++ {
		if _, err := reg.placeRoom(roomIDForTest(i)); err != nil {
			t.Fatalf("place %d: %v", i, err)
		}
	}
	for _, id := range []string{"n1", "n2", "n3"} {
		if got := len(reg.roomsOf(id)); got != 10 {
			t.Fatalf("node %s holds %d rooms, want 10 (even spread)", id, got)
		}
	}
}

func roomIDForTest(i int) string {
	return "room-load-" + string(rune('a'+i%26)) + "-" + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestClusterDCascadeMultiWorkerDelivers(t *testing.T) {
	cascade := newClusterDCascade()
	wa, wb := newClusterDWorker("w-a"), newClusterDWorker("w-b")
	cascade.addWorker(wa)
	cascade.addWorker(wb)
	if err := cascade.placeRoom("room-c", "w-b", 64); err != nil {
		t.Fatal(err)
	}
	// Publish 20 packets from worker A into a room owned by worker B.
	for seq := uint16(0); seq < 20; seq++ {
		payload := []byte{0x65, byte(seq), 0x00, 0x01}
		if err := cascade.publish(clusterDRelayPacket{RoomID: "room-c", TrackID: "track-1", Seq: seq, Payload: payload, KeyFrame: seq == 0}); err != nil {
			t.Fatalf("publish seq %d: %v", seq, err)
		}
	}
	got := wb.drainRoom("room-c")
	if len(got) != 20 {
		t.Fatalf("worker B received %d packets, want 20", len(got))
	}
	for i, pkt := range got {
		if pkt.Seq != uint16(i) {
			t.Fatalf("packet %d out of order: seq %d", i, pkt.Seq)
		}
		if len(pkt.Payload) != 4 || pkt.Payload[1] != byte(i) {
			t.Fatalf("packet %d payload changed in relay", i)
		}
		if i == 0 && !pkt.KeyFrame {
			t.Fatal("first packet lost its keyframe flag in relay")
		}
	}
	// Unknown room must error, not drop silently.
	if err := cascade.publish(clusterDRelayPacket{RoomID: "no-such-room", Seq: 99}); err == nil {
		t.Fatal("publish to unknown room must fail")
	}
}

func TestClusterDFailureDetectionAndRecovery(t *testing.T) {
	reg := newClusterDNodeRegistry()
	base := time.Now()
	reg.now = func() time.Time { return base }
	if err := reg.register("node-a", "10.0.0.1:9000", 10, 1000); err != nil {
		t.Fatal(err)
	}
	for _, room := range []string{"room-r1", "room-r2"} {
		if _, err := reg.placeRoom(room); err != nil {
			t.Fatal(err)
		}
	}
	// Both rooms land on node-a (only node). Register node-b as the
	// recovery target, then kill node-a by clock.
	if err := reg.register("node-b", "10.0.0.2:9000", 10, 1000); err != nil {
		t.Fatal(err)
	}
	// Both rooms land on node-a (first node). Move the clock past
	// the dead threshold for node-a only, keep node-b alive.
	reg.now = func() time.Time { return base.Add(31 * time.Second) }
	if err := reg.heartbeat("node-b"); err != nil {
		t.Fatal(err)
	}
	// heartbeat() stamps with reg.now, so node-b is now alive at
	// base+31s while node-a's last beat is base.
	states := reg.health()
	if states["node-a"] != clusterDNodeDead {
		t.Fatalf("node-a state %v, want dead after 31 s without heartbeat", states["node-a"])
	}
	if states["node-b"] != clusterDNodeHealthy {
		t.Fatalf("node-b state %v, want healthy", states["node-b"])
	}
	detectedAt := base.Add(31 * time.Second)
	recovered, err := reg.recoverDeadNode("node-a", detectedAt)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if len(recovered) != 2 {
		t.Fatalf("recovered %d rooms, want 2", len(recovered))
	}
	for _, rec := range recovered {
		if rec.ToNode != "node-b" {
			t.Fatalf("room %s recovered to %s, want node-b", rec.RoomID, rec.ToNode)
		}
		// Agreed recovery time is 30 s from detection; re-placement
		// is in-memory, so it must be far inside it.
		if rec.Recovered > 30*time.Second {
			t.Fatalf("room %s took %s to recover, over the 30 s agreement", rec.RoomID, rec.Recovered)
		}
	}
	if got := reg.roomsOf("node-a"); len(got) != 0 {
		t.Fatalf("dead node-a still holds rooms: %v", got)
	}
}

func TestClusterDReconcileNeverDropsRooms(t *testing.T) {
	reg := newClusterDNodeRegistry()
	if err := reg.register("n1", "10.0.0.1:9000", 5, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.placeRoom("room-x"); err != nil {
		t.Fatal(err)
	}
	// A snapshot that omits n1 marks it dead but keeps its room.
	changed := reg.reconcileNodeState(map[string]time.Time{})
	if len(changed) != 1 || changed[0] != "n1" {
		t.Fatalf("reconcile changed %v, want [n1]", changed)
	}
	if got := reg.roomsOf("n1"); len(got) != 1 || got[0] != "room-x" {
		t.Fatalf("reconcile dropped room-x: %v", got)
	}
}
