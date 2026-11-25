// Cluster C §6 — simulcast profiles, layer selection, bandwidth control.
//
// profiles.go implements job 6.1 (one profile config file), 6.2
// (profile-level-id negotiation), 6.5 (subscriber layer selection from
// the bandwidth estimate), 6.6 (keyframe-aligned switching) and 6.12
// (publisher bitrate control). The three profiles are the preregistered
// ladder from job 16.1: top 720p30 ~1.2 Mbps, middle 360p30, base
// 180p15. The file is JSON (no new dependency); it is read by the SDP
// path (profile-level-id in the fmtp line), by the queues (max_kbps
// caps the sender) and by the caches (keyframe interval per layer).
//
// Code sha (main.go, unchanged by this file): 9b253fafc2a73a251101fd4435c98ec2c6e17b0f07d94594b6d918b2294fd19a
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// simulcastProfile is one rung of the simulcast ladder (job 6.1).
type simulcastProfile struct {
	Name      string `json:"name"`             // "top" | "middle" | "base"
	Width     int    `json:"width"`            // pixels
	Height    int    `json:"height"`           // pixels
	FPS       int    `json:"fps"`              // frames per second
	MaxKbps   int    `json:"max_kbps"`         // sender must not exceed this
	MinKbps   int    `json:"min_kbps"`         // below this the layer is unusable
	KeyframeS int    `json:"keyframe_s"`       // keyframe interval, seconds (job 6.3: 2)
	ProfileID string `json:"profile_level_id"` // H.264 profile-level-id, e.g. 42001f
}

// profileConfig is the single file read by SDP, queues and caches (6.1).
type profileConfig struct {
	Version  int                `json:"version"`
	Profiles []simulcastProfile `json:"profiles"`
}

// defaultProfileConfig is the 6.1 ladder. Top is the 16.1 preregistered
// 720p30 at ~1.2 Mbps; middle 360p30 at ~600 kbps; base 180p15 at ~200 kbps.
func defaultProfileConfig() profileConfig {
	return profileConfig{
		Version: 1,
		Profiles: []simulcastProfile{
			{Name: "top", Width: 1280, Height: 720, FPS: 30, MaxKbps: 1200, MinKbps: 700, KeyframeS: 2, ProfileID: "42001f"},
			{Name: "middle", Width: 640, Height: 360, FPS: 30, MaxKbps: 600, MinKbps: 300, KeyframeS: 2, ProfileID: "42001f"},
			{Name: "base", Width: 320, Height: 180, FPS: 15, MaxKbps: 200, MinKbps: 80, KeyframeS: 2, ProfileID: "42001f"},
		},
	}
}

// loadProfileConfig reads one profile file (6.1). An empty path returns
// the default ladder. Validation fails closed: duplicate names, a
// missing rung, non-monotonic bitrates, fps<=0 or keyframe<=0 are errors.
func loadProfileConfig(path string) (profileConfig, error) {
	if strings.TrimSpace(path) == "" {
		return defaultProfileConfig(), nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return profileConfig{}, fmt.Errorf("read profile config %s: %w", path, err)
	}
	var cfg profileConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return profileConfig{}, fmt.Errorf("parse profile config %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return profileConfig{}, err
	}
	return cfg, nil
}

func (c profileConfig) validate() error {
	if c.Version != 1 {
		return fmt.Errorf("profile config version %d, want 1", c.Version)
	}
	if len(c.Profiles) != 3 {
		return fmt.Errorf("profile config has %d profiles, want exactly 3 (top/middle/base)", len(c.Profiles))
	}
	seen := map[string]bool{}
	for _, p := range c.Profiles {
		if p.Name != "top" && p.Name != "middle" && p.Name != "base" {
			return fmt.Errorf("unknown profile name %q (want top/middle/base)", p.Name)
		}
		if seen[p.Name] {
			return fmt.Errorf("duplicate profile %q", p.Name)
		}
		seen[p.Name] = true
		if p.Width <= 0 || p.Height <= 0 || p.FPS <= 0 || p.MaxKbps <= 0 || p.MinKbps <= 0 || p.KeyframeS <= 0 {
			return fmt.Errorf("profile %q has a non-positive field", p.Name)
		}
		if p.MinKbps >= p.MaxKbps {
			return fmt.Errorf("profile %q: min_kbps %d >= max_kbps %d", p.Name, p.MinKbps, p.MaxKbps)
		}
		if p.ProfileID == "" {
			return fmt.Errorf("profile %q has no profile_level_id", p.Name)
		}
	}
	byName := c.byName()
	if !(byName["top"].MaxKbps > byName["middle"].MaxKbps && byName["middle"].MaxKbps > byName["base"].MaxKbps) {
		return errors.New("profile max_kbps must decrease top > middle > base")
	}
	return nil
}

func (c profileConfig) byName() map[string]simulcastProfile {
	out := map[string]simulcastProfile{}
	for _, p := range c.Profiles {
		out[p.Name] = p
	}
	return out
}

// profile returns the rung by name; ok=false for an unknown name.
func (c profileConfig) profile(name string) (simulcastProfile, bool) {
	p, ok := c.byName()[name]
	return p, ok
}

// sortedByBitrate returns the rungs low-to-high by MaxKbps.
func (c profileConfig) sortedByBitrate() []simulcastProfile {
	out := append([]simulcastProfile(nil), c.Profiles...)
	sort.Slice(out, func(i, j int) bool { return out[i].MaxKbps < out[j].MaxKbps })
	return out
}

// --- 6.2 profile-level-id negotiation --------------------------------------
//
// H.264 profile-level-id is 6 hex digits: profile_idc (2) +
// constraint flags (2) + level_idc (2). The values seen in the wild:
//   - 42001f: Baseline (0x42), no constraints, level 3.1 (0x1f)
//   - 42e01f: Constrained Baseline (0x42, constraints 0xe0), level 3.1
//   - 4d001f: Main, level 3.1; 64001f: High, level 3.1
//
// 42001f and 42e01f are the SAME profile (Baseline) at the same level;
// they differ only in constraint flags, and level-asymmetry-allowed=1
// (already in our fmtp line, registerH264OnlyCodecs) makes them
// interoperable. Negotiating must therefore compare profile+level,
// ignoring constraint flags — a naive string compare would wrongly
// reject a 42e01f client against our 42001f offer.
//
// negotiateProfileLevelID returns the profile-level-id to answer with:
// the highest level both sides support for the same profile, formatted
// with OUR constraint flags. ok=false when no profile matches.
func negotiateProfileLevelID(ours string, offered []string) (string, bool) {
	ourProfile, ourLevel, ok := parseProfileLevelID(ours)
	if !ok {
		return "", false
	}
	bestLevel := -1
	for _, o := range offered {
		profile, level, ok := parseProfileLevelID(o)
		if !ok || profile != ourProfile {
			continue
		}
		if level <= ourLevel && level > bestLevel {
			bestLevel = level
		}
	}
	if bestLevel < 0 {
		return "", false
	}
	// Answer with our constraint flags (ours[2:4]) and the agreed level.
	return fmt.Sprintf("%02x%s%02x", ourProfile, ours[2:4], bestLevel), true
}

func parseProfileLevelID(s string) (profile, level int, ok bool) {
	if len(s) != 6 {
		return 0, 0, false
	}
	var v int64
	for _, c := range s {
		v *= 16
		switch {
		case c >= '0' && c <= '9':
			v += int64(c - '0')
		case c >= 'a' && c <= 'f':
			v += int64(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v += int64(c-'A') + 10
		default:
			return 0, 0, false
		}
	}
	profile = int((v >> 16) & 0xff)
	level = int(v & 0xff)
	return profile, level, true
}

// --- 6.5 / 7.6 subscriber layer selection ----------------------------------
//
// selectLayerForBandwidth picks the highest rung whose MinKbps the
// estimate can sustain. A capped subscriber therefore drops to a lower
// layer instead of stalling (6.5); the same function is the 7.6 cap
// check at 500 kbps / 1 Mbps / 2 Mbps. A rung may run below its
// MaxKbps (the bitrate controller in 6.12 throttles it); what matters
// is that the estimate covers its minimum usable bitrate.
// Hysteresis: the caller should only switch down immediately and
// switch up after the estimate has held for a full GOP (see layerSwitch
// in adaptation.go).
func selectLayerForBandwidth(cfg profileConfig, estimateKbps int) simulcastProfile {
	if estimateKbps <= 0 {
		return cfg.byName()["base"]
	}
	best := cfg.byName()["base"]
	for _, p := range cfg.Profiles {
		if estimateKbps >= p.MinKbps && p.MaxKbps >= best.MaxKbps {
			best = p
		}
	}
	return best
}

// --- 6.6 keyframe-aligned switching ----------------------------------------
//
// A layer switch may only take effect at a keyframe (the start of a
// GOP). layerSwitch records a pending switch; it becomes effective
// when the next keyframe of the TARGET layer arrives. switchGapOK
// checks the Done line: no gap longer than one GOP (2 s here).
type layerSwitch struct {
	from, to  string
	requested time.Time
	effective time.Time // zero until a keyframe makes it effective
}

// requestLayerSwitch starts a pending switch.
func requestLayerSwitch(from, to string, now time.Time) *layerSwitch {
	return &layerSwitch{from: from, to: to, requested: now}
}

// applyAtKeyframe makes the pending switch effective at a keyframe of
// the target layer. Returns true when the switch took effect.
func (s *layerSwitch) applyAtKeyframe(isKeyframe bool, now time.Time) bool {
	if s == nil || !s.effective.IsZero() {
		return false
	}
	if !isKeyframe {
		return false
	}
	s.effective = now
	return true
}

// switchGapOK reports whether the gap between request and effect is
// at most one GOP (job 6.6 Done line).
func (s *layerSwitch) switchGapOK(gop time.Duration) bool {
	if s == nil || s.effective.IsZero() {
		return false
	}
	return !s.effective.After(s.requested.Add(gop))
}

// --- 6.12 publisher bitrate control ----------------------------------------
//
// bitrateController caps a publisher at its profile max_kbps and
// reduces the target under congestion feedback. It is a leaky-bucket
// rate check plus a multiplicative decrease: on congestion the target
// drops to 80% (floor: profile MinKbps); on a clean interval it
// recovers by 5% per interval (additive increase), never above max.
// All state is in-process and deterministic, so the 1 Mbps-cap test
// (6.12) runs without a real encoder.
type bitrateController struct {
	profile simulcastProfile
	target  int // kbps currently allowed
	// bucket accounting for the current second
	windowStart   time.Time
	bytesInWindow int64
}

func newBitrateController(p simulcastProfile, now time.Time) *bitrateController {
	return &bitrateController{profile: p, target: p.MaxKbps, windowStart: now}
}

// allow reports whether sending nBytes now stays within the target
// for the current one-second window.
func (b *bitrateController) allow(nBytes int64, now time.Time) bool {
	if now.Sub(b.windowStart) >= time.Second {
		b.windowStart = now
		b.bytesInWindow = 0
	}
	return b.bytesInWindow+nBytes <= int64(b.target)*125 // kbps -> bytes/s
}

// sent records bytes actually sent.
func (b *bitrateController) sent(nBytes int64, now time.Time) {
	if now.Sub(b.windowStart) >= time.Second {
		b.windowStart = now
		b.bytesInWindow = 0
	}
	b.bytesInWindow += nBytes
}

// onCongestion reduces the target (multiplicative decrease, floored
// at the profile minimum). Returns the new target.
func (b *bitrateController) onCongestion() int {
	b.target = b.target * 4 / 5
	if b.target < b.profile.MinKbps {
		b.target = b.profile.MinKbps
	}
	return b.target
}

// onCleanInterval recovers the target (additive increase, capped at max).
func (b *bitrateController) onCleanInterval() int {
	b.target = b.target + b.profile.MaxKbps/20
	if b.target > b.profile.MaxKbps {
		b.target = b.profile.MaxKbps
	}
	return b.target
}

// targetKbps returns the current allowed bitrate.
func (b *bitrateController) targetKbps() int { return b.target }
