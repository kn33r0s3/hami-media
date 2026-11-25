package main

import (
	"fmt"
	"os/exec"
)

// runFFmpegTestClip generates a 1 s 320x180 H.264 MP4 test clip.
func runFFmpegTestClip(path string) error {
	cmd := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x180:rate=30",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-g", "30", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg test clip: %w: %s", err, string(out))
	}
	return nil
}

// probeMP4Plays checks ffprobe can read the file's streams.
func probeMP4Plays(path string) error {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type",
		"-of", "csv=p=0", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffprobe: %w: %s", err, string(out))
	}
	if len(out) == 0 {
		return fmt.Errorf("ffprobe found no streams in %s", path)
	}
	return nil
}
