// Command hami-packager is the standalone HLS/CMAF packager process
// (job 13.8): it reads Annex-B H.264 from stdin or a file and writes
// an fMP4 init segment, media segments and playlists to an output
// directory. It is a separate process from the forwarder: the
// forwarder binary does not import this package.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kn33r0s3/hami-media/outputs"
)

func main() {
	outDir := flag.String("out", "", "output directory for init.mp4, seg-*.m4s and media.m3u8 (required)")
	width := flag.Int("width", 640, "video width")
	height := flag.Int("height", 360, "video height")
	framesPerSegment := flag.Int("frames-per-segment", 60, "frames per segment (segments close on keyframes)")
	flag.Parse()
	if *outDir == "" || flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: hami-packager -out <dir> <annexb.h264>")
		os.Exit(2)
	}
	if *width <= 0 || *width > 0xffff || *height <= 0 || *height > 0xffff {
		fmt.Fprintln(os.Stderr, "width and height must be between 1 and 65535")
		os.Exit(2)
	}
	raw, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "read input: %v\n", err)
		os.Exit(1)
	}
	frames, sps, pps, err := outputs.ParseAnnexB(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse Annex-B: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}
	initSeg, err := outputs.InitSegment(sps, pps, uint16(*width), uint16(*height), 90000)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init segment: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "init.mp4"), initSeg, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write init: %v\n", err)
		os.Exit(1)
	}
	p, err := outputs.NewPackager(*framesPerSegment)
	if err != nil {
		fmt.Fprintf(os.Stderr, "packager: %v\n", err)
		os.Exit(1)
	}
	for _, fr := range frames {
		p.Add(fr)
	}
	segments, dropped := p.Close()
	var infos []outputs.SegmentInfo
	baseTime := uint64(0)
	for i, seg := range segments {
		name := fmt.Sprintf("seg-%d.m4s", i)
		data, err := outputs.MediaSegment(seg, uint32(i+1), baseTime)
		if err != nil {
			fmt.Fprintf(os.Stderr, "segment %d: %v\n", i, err)
			os.Exit(1)
		}
		if err := os.WriteFile(filepath.Join(*outDir, name), data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write segment: %v\n", err)
			os.Exit(1)
		}
		var dur uint32
		for _, fr := range seg {
			dur += fr.Duration
		}
		infos = append(infos, outputs.SegmentInfo{Filename: name, DurationSecs: float64(dur) / 90000, KeyFrame: true, Frames: len(seg)})
		baseTime += uint64(dur)
	}
	playlist := outputs.MediaPlaylist(infos, "init.mp4", maxSegmentDuration(infos))
	if err := os.WriteFile(filepath.Join(*outDir, "media.m3u8"), []byte(playlist), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write playlist: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("packaged %d segments (%d frames dropped before first keyframe) into %s\n", len(infos), dropped, *outDir)
}

func maxSegmentDuration(infos []outputs.SegmentInfo) float64 {
	max := 0.0
	for _, info := range infos {
		if info.DurationSecs > max {
			max = info.DurationSecs
		}
	}
	return max
}
