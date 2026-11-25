// Cluster C §9.1 — signaling spec with versioning.
//
// The spec document is docs/cluster-c-signaling-spec.md (version 1).
// This file is the implementation side: parseSignalingMessage
// validates every incoming message against the spec (version, type,
// required fields, size limit) and the tests in
// cluster_c_section9_test.go run the implementation against the spec.
// Unknown versions and unknown types are rejected, never ignored.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// signalingVersion is the current spec version. Version 0 / missing
// version is rejected; a future version is rejected (no silent
// downgrade).
const signalingVersion = 1

// signalingMessage is one validated signaling message.
type signalingMessage struct {
	Version int             `json:"version"`
	Type    string          `json:"type"`
	Room    string          `json:"room,omitempty"`
	From    string          `json:"from,omitempty"`
	To      string          `json:"to,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// signalingTypes is the spec's message set (§9.1).
var signalingTypes = map[string]bool{
	"join": true, "leave": true, "offer": true, "answer": true,
	"candidate": true, "publish": true, "subscribe": true,
	"layer-switch": true, "metadata": true, "message": true,
	"resume": true, "error": true,
}

// parseSignalingMessage validates raw JSON against the spec.
func parseSignalingMessage(raw []byte) (signalingMessage, error) {
	if len(raw) > maxSignalingJSONBytes {
		return signalingMessage{}, fmt.Errorf("signaling message %d bytes exceeds limit %d", len(raw), maxSignalingJSONBytes)
	}
	var msg signalingMessage
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&msg); err != nil {
		return signalingMessage{}, fmt.Errorf("signaling message is not valid JSON: %w", err)
	}
	if msg.Version != signalingVersion {
		return signalingMessage{}, fmt.Errorf("signaling version %d, want %d", msg.Version, signalingVersion)
	}
	if !signalingTypes[msg.Type] {
		return signalingMessage{}, fmt.Errorf("unknown signaling type %q", msg.Type)
	}
	switch msg.Type {
	case "join", "leave", "publish", "subscribe", "resume":
		if strings.TrimSpace(msg.Room) == "" {
			return signalingMessage{}, fmt.Errorf("signaling %s needs a room", msg.Type)
		}
		if strings.TrimSpace(msg.From) == "" {
			return signalingMessage{}, fmt.Errorf("signaling %s needs a sender", msg.Type)
		}
	case "offer", "answer", "candidate":
		if strings.TrimSpace(msg.From) == "" || strings.TrimSpace(msg.To) == "" {
			return signalingMessage{}, fmt.Errorf("signaling %s needs from and to", msg.Type)
		}
		if len(msg.Payload) == 0 {
			return signalingMessage{}, fmt.Errorf("signaling %s needs a payload", msg.Type)
		}
	case "layer-switch", "metadata", "message":
		if strings.TrimSpace(msg.Room) == "" || strings.TrimSpace(msg.From) == "" {
			return signalingMessage{}, fmt.Errorf("signaling %s needs room and from", msg.Type)
		}
	}
	return msg, nil
}

// signalingError builds a spec error message (type "error").
func signalingError(room, to, code, detail string) (signalingMessage, error) {
	if strings.TrimSpace(code) == "" {
		return signalingMessage{}, errors.New("signaling error needs a code")
	}
	payload, err := json.Marshal(map[string]string{"code": code, "detail": detail})
	if err != nil {
		return signalingMessage{}, err
	}
	return signalingMessage{Version: signalingVersion, Type: "error", Room: room, To: to, Payload: payload}, nil
}
