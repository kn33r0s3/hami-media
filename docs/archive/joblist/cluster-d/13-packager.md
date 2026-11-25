# 13.1/13.2 HLS/CMAF packager — status and verification

## What was built

hami-media's OWN packager (package `outputs`, `outputs/packager.go`):
fMP4 init segment (ftyp + moov with avcC), media segments (moof +
mdat with tfdt/trun, data-offset pointing at mdat), HLS media
playlist (EXT-X-VERSION:7, EXT-X-MAP) and master playlist. A
standalone packager process: `cmd/hami-packager` (13.8 — the
forwarder builds and tests without it).

This is NOT MediaMTX: MediaMTX remux/packaging results do not count
for 13.1/13.2, and none are cited here.

## 13.2 — every segment starts on a keyframe

Enforced twice: `MediaSegment` refuses a non-keyframe first frame,
and `Packager.Close` drops leading non-keyframes and splits only on
keyframe boundaries. Test: `TestPackagerSegmentsAreKeyframeAligned`
+ `TestMediaSegmentStartsOnKeyframe`.

## Verification (VM)

1. Unit tests: `outputs` package — init segment shape, segment
   keyframe rule, trun data-offset, playlist contents.
2. End-to-end: `cmd/hami-packager` packages a real H.264 file
   (ffmpeg-generated test clip) into init.mp4 + seg-*.m4s +
   media.m3u8; `ffprobe` reads the playlist and `ffmpeg` decodes
   it. Evidence: `log (sha256 e5f89b6f0442, 48 lines; kept outside repo)`.

## Gaps (honest)

- **Safari playback cannot be verified in this VM** (no Safari, no
  browser at all). The Done line wants playback in Safari AND
  hls.js. hls.js playback likewise needs a browser. What is verified
  here is: standards-conformant fMP4 segments + playlists that
  ffmpeg/ffprobe accept and decode, which is the same byte stream a
  browser player consumes. Safari + hls.js playback remain owed on
  a machine with a browser.
- **hls.js in the VM**: not possible without a browser; not faked.
