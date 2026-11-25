# 13.7 Ingest — WHIP (RTMP not implemented)

## Decision: WHIP only

WHIP (RFC 9444, WebRTC-HTTP Ingestion Protocol) is the ingest.
An encoder POSTs an SDP offer to `/whip/{room}` with
`Content-Type: application/sdp` and gets an SDP answer (201).
The encoder then publishes WebRTC media into the room.

RTMP ingest is NOT implemented. Reasons:
- WHIP is WebRTC-native: no separate RTMP server, no second media
  protocol to operate, and the encoder (OBS 30+, ffmpeg 7+) speaks
  WHIP directly.
- RTMP would need a full RTMP server (handshake, chunking, AMF) for
  one legacy protocol; that is a separate job if a customer needs it.

## Encoder used in the test

OBS is not installed in the VM, so the encoder in the evidence run
is **ffmpeg's WHIP muxer** (`ffmpeg -f whip`), plus a Pion
PeerConnection driving the same endpoint in the Go test
(`TestClusterDWHIPIngestOfferAnswer`). OBS compatibility is the
same protocol (WHIP); OBS itself was not run here — recorded, not
claimed.

## Evidence

- Go test: WHIP offer/answer handshake + rejection of wrong
  content-type / missing room.
- ffmpeg WHIP attempt: see `log (sha256 596121afc5ad, 8 lines; kept outside repo)`
  (VM UDP block applies to the media path; the HTTP handshake is
  the gate that can pass here).
