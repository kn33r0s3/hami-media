// Cluster C §6.9 — codec set per room.
//
// registerH264OnlyCodecs (main.go) registers H.264 only. Job 6.9's Done
// line is: each of H.264, VP8, VP9 and AV1 is either tested end to end
// or listed as unsupported in the docs. This file is the tested part:
// H.264 is the only supported codec; VP8, VP9 and AV1 are explicitly
// unsupported, and codecForRoom refuses them with a named error instead
// of silently falling back. The doc list lives in
// docs/cluster-c-codecs.md. No VP8/VP9/AV1 encoder or decoder is
// registered anywhere in this tree.
package main

import (
	"fmt"
	"strings"
)

// supportedCodecs is the codec set this build can forward.
var supportedCodecs = []string{"H264"}

// unsupportedCodecs lists the job-6.9 codecs that are NOT supported,
// with the reason. Tested end to end: H.264 only.
var unsupportedCodecs = map[string]string{
	"VP8": "not registered: registerH264OnlyCodecs registers H.264 only; no VP8 encoder/decoder in this build",
	"VP9": "not registered: registerH264OnlyCodecs registers H.264 only; no VP9 encoder/decoder in this build",
	"AV1": "not registered: registerH264OnlyCodecs registers H.264 only; no AV1 encoder/decoder in this build",
}

// codecForRoom resolves the codec for a room's profile. codec is the
// requested codec name (case-insensitive, "H264"/"video/H264" forms
// accepted). An unsupported codec is an error, never a silent fallback.
func codecForRoom(room, codec string) (string, error) {
	if strings.TrimSpace(room) == "" {
		return "", fmt.Errorf("codec selection needs a room name")
	}
	name := strings.ToUpper(strings.TrimSpace(codec))
	name = strings.TrimPrefix(name, "VIDEO/")
	if name == "H264" {
		return "H264", nil
	}
	if reason, ok := unsupportedCodecs[name]; ok {
		return "", fmt.Errorf("codec %s unsupported for room %q: %s", name, room, reason)
	}
	return "", fmt.Errorf("unknown codec %q for room %q", codec, room)
}

// isSupportedCodec reports whether the codec can be forwarded.
func isSupportedCodec(codec string) bool {
	_, err := codecForRoom("default", codec)
	return err == nil
}
