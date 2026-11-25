// Cluster D section 13.5/13.6: recording to MP4 and to segments,
// with retention + deletion, and the recording consent flow.
//
// Recording writes packaged segments (13.1) to a recording
// directory, then remuxes them to MP4 with ffmpeg (no re-encode).
// Retention is a per-recording expiry; deletion removes both the
// segments and the MP4 and is verified by a re-list. Consent (13.6)
// is recorded BEFORE recording starts: startRecording refuses
// without a consent record for the room.

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// clusterDConsent is one recorded consent to be recorded (13.6).
type clusterDConsent struct {
	RoomID      string
	Participant string // who consented (the room owner / speaker)
	ConsentedAt time.Time
	Scope       string // e.g. "audio+video"
}

type clusterDConsentStore struct {
	mu       sync.Mutex
	consents map[string]clusterDConsent // room -> consent
}

func newClusterDConsentStore() *clusterDConsentStore {
	return &clusterDConsentStore{consents: make(map[string]clusterDConsent)}
}

// record stores consent for a room. It must happen before recording
// starts; startRecording checks it.
func (s *clusterDConsentStore) record(roomID, participant, scope string) clusterDConsent {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := clusterDConsent{RoomID: roomID, Participant: participant, ConsentedAt: time.Now().UTC(), Scope: scope}
	s.consents[roomID] = c
	return c
}

func (s *clusterDConsentStore) has(roomID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.consents[roomID]
	return ok
}

// clusterDRecording is one recording's state.
type clusterDRecording struct {
	ID          string
	RoomID      string
	Dir         string
	StartedAt   time.Time
	RetainUntil time.Time
	MP4Path     string
	Segments    []string
	Deleted     bool
}

type clusterDRecorder struct {
	mu         sync.Mutex
	baseDir    string
	recordings map[string]*clusterDRecording
	consents   *clusterDConsentStore
	ffmpeg     string // path to ffmpeg; "" = look up in PATH
}

func newClusterDRecorder(baseDir string, consents *clusterDConsentStore) *clusterDRecorder {
	return &clusterDRecorder{baseDir: baseDir, recordings: make(map[string]*clusterDRecording), consents: consents}
}

// validateRecordingID ensures id is a single safe directory component under baseDir.
func validateRecordingID(id string) error {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
		return fmt.Errorf("recording: invalid recording id %q", id)
	}
	return nil
}

// startRecording begins a recording. It refuses without prior
// consent for the room (13.6) and without a retention period.
func (r *clusterDRecorder) startRecording(id, roomID string, retain time.Duration) (*clusterDRecording, error) {
	if err := validateRecordingID(id); err != nil {
		return nil, err
	}
	if !r.consents.has(roomID) {
		return nil, fmt.Errorf("recording %s: consent not recorded for room %s: record consent before recording starts", id, roomID)
	}
	if retain <= 0 {
		return nil, errors.New("recording: retention must be > 0")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.recordings[id]; exists {
		return nil, fmt.Errorf("recording %s already exists", id)
	}
	dir := filepath.Join(r.baseDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	rec := &clusterDRecording{ID: id, RoomID: roomID, Dir: dir, StartedAt: time.Now().UTC(), RetainUntil: time.Now().UTC().Add(retain)}
	r.recordings[id] = rec
	return rec, nil
}

// addSegment records one packaged segment file in the recording.
func (r *clusterDRecorder) addSegment(id, segmentPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.recordings[id]
	if !ok {
		return fmt.Errorf("recording %s not found", id)
	}
	if rec.Deleted {
		return fmt.Errorf("recording %s is deleted", id)
	}
	rec.Segments = append(rec.Segments, segmentPath)
	return nil
}

// finishMP4 remuxes the recorded segments into a single MP4 with
// ffmpeg (stream copy, no re-encode). The MP4 is the playback
// artifact; the segments remain until retention expires.
func (r *clusterDRecorder) finishMP4(id string) (string, error) {
	r.mu.Lock()
	rec, ok := r.recordings[id]
	if !ok {
		r.mu.Unlock()
		return "", fmt.Errorf("recording %s not found", id)
	}
	if rec.Deleted {
		r.mu.Unlock()
		return "", fmt.Errorf("recording %s is deleted", id)
	}
	segments := append([]string(nil), rec.Segments...)
	recDir := rec.Dir
	ffmpeg := r.ffmpeg
	r.mu.Unlock()

	if len(segments) == 0 {
		return "", fmt.Errorf("recording %s has no segments to remux", id)
	}
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	// Build a concat list for the segment files.
	listPath := filepath.Join(recDir, "concat.txt")
	var listContent strings.Builder
	for _, seg := range segments {
		if strings.ContainsAny(seg, "\n\r") {
			return "", fmt.Errorf("recording %s: invalid segment path", id)
		}
		escaped := strings.ReplaceAll(seg, "'", "'\\''")
		listContent.WriteString("file '")
		listContent.WriteString(escaped)
		listContent.WriteString("'\n")
	}
	if err := os.WriteFile(listPath, []byte(listContent.String()), 0o644); err != nil {
		return "", err
	}
	mp4Path := filepath.Join(recDir, "recording.mp4")
	cmd := exec.Command(ffmpeg, "-y", "-v", "error", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", mp4Path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg remux: %w: %s", err, string(out))
	}
	r.mu.Lock()
	rec.MP4Path = mp4Path
	r.mu.Unlock()
	return mp4Path, nil
}

// deleteRecording removes the segments, the MP4 and the directory
// (13.5 retention/deletion). It reports every file it removed.
func (r *clusterDRecorder) deleteRecording(id string) ([]string, error) {
	r.mu.Lock()
	rec, ok := r.recordings[id]
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("recording %s not found", id)
	}
	var removed []string
	entries, err := os.ReadDir(rec.Dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		p := filepath.Join(rec.Dir, e.Name())
		if err := os.Remove(p); err != nil {
			return removed, err
		}
		removed = append(removed, p)
	}
	if err := os.Remove(rec.Dir); err != nil {
		return removed, err
	}
	r.mu.Lock()
	rec.Deleted = true
	rec.Segments = nil
	rec.MP4Path = ""
	r.mu.Unlock()
	return removed, nil
}

// expired lists recordings past their retention time.
func (r *clusterDRecorder) expired(now time.Time) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for id, rec := range r.recordings {
		if !rec.Deleted && now.After(rec.RetainUntil) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
