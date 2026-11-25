// Cluster D (job list sections 11, 14.4, 14.5): multi-node registry,
// placement, cascading and failure recovery.
//
// Design notes (see docs/cluster-d/11-shared-state-decision.md and
// 11-multi-region-decision.md):
//   - Nodes are registered in-memory with a heartbeat. There is no
//     external shared store in this tree; reconciliation is explicit
//     (see reconcileNodeState) and its failure modes are documented.
//   - Cascading here is the multi-worker in-process form: each worker
//     owns a set of rooms and forwards relay packets to the worker that
//     owns a room. Cross-machine cascade needs a second machine (see
//     11.3 verdict: PARTIAL, cross-machine gap recorded).
//   - Failure detection is heartbeat-based with a stated timeout; room
//     recovery reassigns a dead node's rooms to live nodes and reports
//     how long each room took to become reachable again.

package main

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// clusterDNodeState is the health state of one registered node.
type clusterDNodeState string

const (
	clusterDNodeHealthy      clusterDNodeState = "healthy"
	clusterDNodeSuspect      clusterDNodeState = "suspect"
	clusterDNodeDrained      clusterDNodeState = "drained"
	clusterDNodeDead         clusterDNodeState = "dead"
	clusterDNodeHeartbeat                      = 5 * time.Second  // expected heartbeat interval
	clusterDNodeSuspectAfter                   = 15 * time.Second // no heartbeat -> suspect
	clusterDNodeDeadAfter                      = 30 * time.Second // no heartbeat -> dead
)

// clusterDNode is one node in the registry.
type clusterDNode struct {
	ID         string
	Address    string
	MaxRooms   int
	MaxViewers int
	Rooms      map[string]struct{} // room IDs owned by this node
	Viewers    int
	LastBeat   time.Time
	State      clusterDNodeState
	Drain      bool
}

// clusterDNodeRegistry is the in-memory node registry (11.1).
type clusterDNodeRegistry struct {
	mu    sync.Mutex
	nodes map[string]*clusterDNode
	now   func() time.Time
}

func newClusterDNodeRegistry() *clusterDNodeRegistry {
	return &clusterDNodeRegistry{nodes: make(map[string]*clusterDNode), now: time.Now}
}

func (reg *clusterDNodeRegistry) register(id, address string, maxRooms, maxViewers int) error {
	if id == "" {
		return errors.New("node id is required")
	}
	if maxRooms <= 0 || maxViewers <= 0 {
		return fmt.Errorf("node %s: maxRooms and maxViewers must be > 0", id)
	}
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if _, exists := reg.nodes[id]; exists {
		return fmt.Errorf("node %s already registered", id)
	}
	reg.nodes[id] = &clusterDNode{
		ID: id, Address: address, MaxRooms: maxRooms, MaxViewers: maxViewers,
		Rooms: make(map[string]struct{}), LastBeat: reg.now(), State: clusterDNodeHealthy,
	}
	return nil
}

func (reg *clusterDNodeRegistry) heartbeat(id string) error {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	node, ok := reg.nodes[id]
	if !ok {
		return fmt.Errorf("node %s not registered", id)
	}
	node.LastBeat = reg.now()
	if node.State == clusterDNodeSuspect {
		node.State = clusterDNodeHealthy
	}
	return nil
}

// health re-evaluates every node's state from its last heartbeat.
func (reg *clusterDNodeRegistry) health() map[string]clusterDNodeState {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	out := make(map[string]clusterDNodeState, len(reg.nodes))
	now := reg.now()
	for _, node := range reg.nodes {
		if node.Drain {
			node.State = clusterDNodeDrained
		} else if now.Sub(node.LastBeat) > clusterDNodeDeadAfter {
			node.State = clusterDNodeDead
		} else if now.Sub(node.LastBeat) > clusterDNodeSuspectAfter {
			node.State = clusterDNodeSuspect
		} else {
			node.State = clusterDNodeHealthy
		}
		out[node.ID] = node.State
	}
	return out
}

// drain marks a node drained (11.1). A drained node receives no new
// rooms; existing rooms stay until they are moved by recovery.
func (reg *clusterDNodeRegistry) drain(id string) error {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	node, ok := reg.nodes[id]
	if !ok {
		return fmt.Errorf("node %s not registered", id)
	}
	node.Drain = true
	node.State = clusterDNodeDrained
	return nil
}

func (reg *clusterDNodeRegistry) addRoom(nodeID, roomID string) error {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	node, ok := reg.nodes[nodeID]
	if !ok {
		return fmt.Errorf("node %s not registered", nodeID)
	}
	if node.Drain {
		return fmt.Errorf("node %s is drained: refuses new room %s", nodeID, roomID)
	}
	if len(node.Rooms) >= node.MaxRooms {
		return fmt.Errorf("node %s is at room capacity %d", nodeID, node.MaxRooms)
	}
	node.Rooms[roomID] = struct{}{}
	return nil
}

func (reg *clusterDNodeRegistry) roomsOf(nodeID string) []string {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	node, ok := reg.nodes[nodeID]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(node.Rooms))
	for room := range node.Rooms {
		out = append(out, room)
	}
	sort.Strings(out)
	return out
}

// placeRoom picks a node for a new room (11.2): the healthy,
// non-drained node with the most remaining room capacity, breaking
// ties by fewest viewers. Under load the placement spreads rooms.
func (reg *clusterDNodeRegistry) placeRoom(roomID string) (string, error) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	now := reg.now()
	var best *clusterDNode
	bestFree := -1
	for _, node := range reg.nodes {
		if node.Drain || node.State == clusterDNodeDead {
			continue
		}
		if now.Sub(node.LastBeat) > clusterDNodeDeadAfter {
			continue
		}
		free := node.MaxRooms - len(node.Rooms)
		if free <= 0 {
			continue
		}
		if free > bestFree ||
			(free == bestFree && best != nil && (node.Viewers < best.Viewers || (node.Viewers == best.Viewers && node.ID < best.ID))) {
			best, bestFree = node, free
		}
	}
	if best == nil {
		return "", fmt.Errorf("no healthy node with capacity for room %s", roomID)
	}
	best.Rooms[roomID] = struct{}{}
	return best.ID, nil
}

// reconcileNodeState re-derives registry state from a fresh snapshot
// of node heartbeats (the reconciliation step of the in-memory +
// reconciliation shared-state decision, 11.5). A node absent from the
// snapshot keeps its rooms in the registry but is marked dead; the
// caller must run recovery before those rooms are re-placed, so a
// stale snapshot can never silently drop a room.
func (reg *clusterDNodeRegistry) reconcileNodeState(snapshot map[string]time.Time) []string {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	var changed []string
	now := reg.now()
	for id, node := range reg.nodes {
		beat, seen := snapshot[id]
		if !seen {
			if node.State != clusterDNodeDead {
				node.State = clusterDNodeDead
				changed = append(changed, id)
			}
			continue
		}
		node.LastBeat = beat
		if now.Sub(beat) > clusterDNodeDeadAfter {
			node.State = clusterDNodeDead
		} else if node.Drain {
			node.State = clusterDNodeDrained
		} else {
			node.State = clusterDNodeHealthy
		}
	}
	return changed
}

// clusterDRoomRecovery is one room's recovery outcome (11.4).
type clusterDRoomRecovery struct {
	RoomID    string
	FromNode  string
	ToNode    string
	Recovered time.Duration // time from detection to reachable again
}

// recoverDeadNode moves every room of a dead node to healthy nodes
// (11.4). The agreed recovery time is 30 s (the dead-after timeout);
// the test asserts each room is re-placed well inside it because
// re-placement itself is in-memory and immediate once detection fires.
func (reg *clusterDNodeRegistry) recoverDeadNode(deadID string, detectedAt time.Time) ([]clusterDRoomRecovery, error) {
	reg.mu.Lock()
	dead, ok := reg.nodes[deadID]
	if !ok {
		reg.mu.Unlock()
		return nil, fmt.Errorf("node %s not registered", deadID)
	}
	rooms := make([]string, 0, len(dead.Rooms))
	for room := range dead.Rooms {
		rooms = append(rooms, room)
	}
	sort.Strings(rooms)
	reg.mu.Unlock()

	var recovered []clusterDRoomRecovery
	for _, room := range rooms {
		to, err := reg.placeRoom(room)
		if err != nil {
			return recovered, fmt.Errorf("room %s: no live node to recover to: %w", room, err)
		}
		recovered = append(recovered, clusterDRoomRecovery{
			RoomID: room, FromNode: deadID, ToNode: to, Recovered: time.Since(detectedAt),
		})
	}
	reg.mu.Lock()
	delete(dead.Rooms, "")
	for _, room := range rooms {
		delete(dead.Rooms, room)
	}
	reg.mu.Unlock()
	return recovered, nil
}

// ---- Multi-worker in-process cascade (11.3) ----

// clusterDWorker is one cascade worker: it owns rooms and forwards
// relay packets to the worker owning a room.
type clusterDWorker struct {
	id       string
	rooms    map[string]chan clusterDRelayPacket
	mu       sync.Mutex
	received int64
	relayed  int64
}

// clusterDRelayPacket is one packet relayed between cascade workers.
type clusterDRelayPacket struct {
	RoomID   string
	TrackID  string
	Seq      uint16
	Payload  []byte // copied on relay; payload bytes must be unchanged
	KeyFrame bool
}

func newClusterDWorker(id string) *clusterDWorker {
	return &clusterDWorker{id: id, rooms: make(map[string]chan clusterDRelayPacket)}
}

func (w *clusterDWorker) ownRoom(roomID string, buf int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.rooms[roomID] = make(chan clusterDRelayPacket, buf)
}

// relay delivers a packet to the worker owning roomID. It returns an
// error instead of dropping silently.
func (w *clusterDWorker) relay(pkt clusterDRelayPacket) error {
	w.mu.Lock()
	ch, ok := w.rooms[pkt.RoomID]
	w.mu.Unlock()
	if !ok {
		return fmt.Errorf("worker %s does not own room %s", w.id, pkt.RoomID)
	}
	cp := make([]byte, len(pkt.Payload))
	copy(cp, pkt.Payload)
	pkt.Payload = cp
	select {
	case ch <- pkt:
		w.mu.Lock()
		w.relayed++
		w.mu.Unlock()
		return nil
	default:
		return fmt.Errorf("worker %s room %s relay queue full", w.id, pkt.RoomID)
	}
}

// drainRoom returns every packet delivered to a room so far.
func (w *clusterDWorker) drainRoom(roomID string) []clusterDRelayPacket {
	w.mu.Lock()
	ch, ok := w.rooms[roomID]
	w.mu.Unlock()
	if !ok {
		return nil
	}
	var out []clusterDRelayPacket
	for {
		select {
		case pkt := <-ch:
			w.mu.Lock()
			w.received++
			w.mu.Unlock()
			out = append(out, pkt)
		default:
			return out
		}
	}
}

// clusterDCascade links workers into one cascade: publish on any
// worker, subscribe on the worker owning the room.
type clusterDCascade struct {
	workers map[string]*clusterDWorker
	owner   map[string]string // room -> worker id
	mu      sync.Mutex
}

func newClusterDCascade() *clusterDCascade {
	return &clusterDCascade{workers: make(map[string]*clusterDWorker), owner: make(map[string]string)}
}

func (c *clusterDCascade) addWorker(w *clusterDWorker) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.workers[w.id] = w
}

func (c *clusterDCascade) placeRoom(roomID, workerID string, buf int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	w, ok := c.workers[workerID]
	if !ok {
		return fmt.Errorf("cascade worker %s unknown", workerID)
	}
	w.ownRoom(roomID, buf)
	c.owner[roomID] = workerID
	return nil
}

// publish forwards a packet from any worker to the room owner.
func (c *clusterDCascade) publish(pkt clusterDRelayPacket) error {
	c.mu.Lock()
	ownerID, ok := c.owner[pkt.RoomID]
	owner, wok := c.workers[ownerID]
	c.mu.Unlock()
	if !ok || !wok {
		return fmt.Errorf("cascade: no owner for room %s", pkt.RoomID)
	}
	return owner.relay(pkt)
}
