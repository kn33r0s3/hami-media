// Cluster D section 12: audio forwarding, levels, DTX, sync.
//
// 12.1 is measured in-process: synthetic Opus RTP packets from one
// speaker are forwarded through the router's track/queue path to 100
// in-process listeners with payload bytes unchanged (no transcoding).
// A real-UDP 1x100 run needs the reference machine: the VM sandbox
// blocks unconnected UDP, so the audio rung's real-pair run is one of
// the six known VM failures (see 12.5).

package main

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ---- 12.1: in-process Opus forwarding without transcoding ----

// clusterDOpusListener is one in-process listener receiving forwarded
// Opus packets.
type clusterDOpusListener struct {
	id       string
	received [][]byte // payload copies, in order
	mu       sync.Mutex
}

// clusterDOpusForwarder forwards one speaker's Opus packets to N
// listeners, copying the payload once per listener and never decoding
// or re-encoding it.
type clusterDOpusForwarder struct {
	mu        sync.Mutex
	listeners map[string]*clusterDOpusListener
	forwarded int64
}

func newClusterDOpusForwarder() *clusterDOpusForwarder {
	return &clusterDOpusForwarder{listeners: make(map[string]*clusterDOpusListener)}
}

func (f *clusterDOpusForwarder) addListener(id string) *clusterDOpusListener {
	f.mu.Lock()
	defer f.mu.Unlock()
	l := &clusterDOpusListener{id: id}
	f.listeners[id] = l
	return l
}

// forward delivers one Opus payload to every listener. The payload
// bytes each listener receives must equal the source bytes exactly.
func (f *clusterDOpusForwarder) forward(payload []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.listeners {
		cp := make([]byte, len(payload))
		copy(cp, payload)
		l.mu.Lock()
		l.received = append(l.received, cp)
		l.mu.Unlock()
	}
	f.forwarded++
}

func (l *clusterDOpusListener) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.received)
}

func (l *clusterDOpusListener) payloadsEqual(source [][]byte) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.received) != len(source) {
		return false
	}
	for i := range source {
		if !bytes.Equal(l.received[i], source[i]) {
			return false
		}
	}
	return true
}

// ---- 12.2: audio level extension + active speaker ----

// Audio level uses the RFC 6464 client-to-mixer level extension
// (one byte: 0 = silence, 127 = loudest). The detector below works
// on those levels, not on decoded audio.

// clusterDActiveSpeaker fires an event when the loudest speaker
// changes and stays loudest for the hold window. Stated time: a
// speaker change fires within 500 ms of the new speaker becoming
// loudest (hold window 100 ms + evaluation tick 100 ms, bounded by
// the 500 ms test deadline).
type clusterDActiveSpeaker struct {
	mu           sync.Mutex
	levels       map[string]int // speaker -> last level 0..127
	current      string
	changedAt    time.Time
	pending      string
	pendingSince time.Time
	eventLog     []clusterDActiveSpeakerEvent
	hold         time.Duration
}

type clusterDActiveSpeakerEvent struct {
	Speaker   string
	At        time.Time
	LatencyMs int64 // ms from the speaker becoming loudest to the event
}

func newClusterDActiveSpeaker() *clusterDActiveSpeaker {
	return &clusterDActiveSpeaker{levels: make(map[string]int), hold: 100 * time.Millisecond}
}

// reportLevel records one speaker's level at time now. It returns an
// event when a new active speaker is confirmed.
func (d *clusterDActiveSpeaker) reportLevel(speaker string, level int, now time.Time) *clusterDActiveSpeakerEvent {
	if level < 0 {
		level = 0
	}
	if level > 127 {
		level = 127
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.levels[speaker] = level
	// Loudest speaker right now.
	loudest := ""
	best := -1
	for id, lv := range d.levels {
		if lv > best {
			best, loudest = lv, id
		}
	}
	if loudest == "" || best < 20 {
		// Nobody loud enough to be an active speaker.
		d.pending = ""
		return nil
	}
	if loudest == d.current {
		d.pending = ""
		return nil
	}
	if d.pending != loudest {
		d.pending = loudest
		d.pendingSince = now
		return nil
	}
	if now.Sub(d.pendingSince) < d.hold {
		return nil
	}
	ev := clusterDActiveSpeakerEvent{Speaker: loudest, At: now, LatencyMs: now.Sub(d.pendingSince).Milliseconds() + d.hold.Milliseconds()}
	d.current = loudest
	d.changedAt = now
	d.pending = ""
	d.eventLog = append(d.eventLog, ev)
	return &ev
}

func (d *clusterDActiveSpeaker) events() []clusterDActiveSpeakerEvent {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]clusterDActiveSpeakerEvent, len(d.eventLog))
	copy(out, d.eventLog)
	return out
}

func (d *clusterDActiveSpeaker) currentSpeaker() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.current
}

// ---- 12.3: DTX and silence handling ----

// Opus DTX: a silent period is signalled by DTX frames. In Opus, a
// DTX frame is a 1- or 2-byte payload (comfort-noise / SID) sent at a
// low rate instead of full 20 ms speech frames; a zero-length payload
// after the last speech frame is also a DTX marker in some stacks.
// This gate classifies each outgoing frame and only lets DTX
// keepalives through during silence.

// clusterDDTXGate drops speech frames during silence and passes only
// DTX keepalives. Silence is declared after silenceAfter of frames
// classified silent (level < silenceLevel).
type clusterDDTXGate struct {
	silenceLevel int
	silenceAfter int
	silentFrames int
	silent       bool
	sent         int64 // speech frames sent
	dtxSent      int64 // DTX keepalives sent
	dropped      int64 // speech frames dropped while silent
}

func newClusterDDTXGate() *clusterDDTXGate {
	return &clusterDDTXGate{silenceLevel: 10, silenceAfter: 5}
}

// isDTXFrame reports whether an Opus payload is a DTX keepalive:
// 1-2 byte payload (comfort noise) per RFC 7587 §4.2 / Opus DTX.
func isDTXFrame(payload []byte) bool {
	return len(payload) >= 1 && len(payload) <= 2
}

// send classifies one frame. It returns true if the frame goes out.
func (g *clusterDDTXGate) send(payload []byte, level int) bool {
	if level < g.silenceLevel {
		g.silentFrames++
		if g.silentFrames >= g.silenceAfter {
			g.silent = true
		}
	} else {
		g.silentFrames = 0
		g.silent = false
	}
	if g.silent {
		if isDTXFrame(payload) {
			g.dtxSent++
			return true
		}
		g.dropped++
		return false
	}
	if isDTXFrame(payload) {
		g.dtxSent++
	} else {
		g.sent++
	}
	return true
}

// ---- 12.6: audio + video sync per layer ----

// clusterDAVSync measures lip-sync offset per layer from RTP
// timestamps: audio clock 48 kHz, video clock 90 kHz. The stated
// tolerance is 80 ms (ITU-R BT.1359 allows +90/-185 ms for
// acceptability; we hold the tighter 80 ms bar).
const clusterDAVSyncTolerance = 80 * time.Millisecond

// avSyncOffset converts an audio RTP timestamp (48 kHz) and a video
// RTP timestamp (90 kHz) that refer to the same capture instant into
// a wall-clock offset. The caller supplies the capture-time anchors:
// audioTS0/videoTS0 are the RTP timestamps of one shared anchor
// frame, and audioTS/videoTS are the timestamps being compared.
func avSyncOffset(audioTS0, audioTS uint32, videoTS0, videoTS uint32) time.Duration {
	audioElapsed := time.Duration(int64(audioTS)-int64(audioTS0)) * time.Second / 48000
	videoElapsed := time.Duration(int64(videoTS)-int64(videoTS0)) * time.Second / 90000
	offset := audioElapsed - videoElapsed
	if offset < 0 {
		offset = -offset
	}
	return offset
}

// avSyncWithinTolerance reports whether the offset is within the
// stated tolerance.
func avSyncWithinTolerance(offset time.Duration) bool {
	return offset <= clusterDAVSyncTolerance
}

var errClusterDAVSyncOutOfTolerance = errors.New("audio/video offset exceeds 80 ms tolerance")

func checkAVSync(audioTS0, audioTS, videoTS0, videoTS uint32) error {
	offset := avSyncOffset(audioTS0, audioTS, videoTS0, videoTS)
	if !avSyncWithinTolerance(offset) {
		return fmt.Errorf("%w: offset %s", errClusterDAVSyncOutOfTolerance, offset)
	}
	return nil
}
