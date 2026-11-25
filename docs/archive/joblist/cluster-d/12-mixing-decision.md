# 12.4 Server mixing decision

## Decision: mixing runs in-process, as a documented separate mode

The audio mixer exists in-process: `startAudioRTPMixer`
(`main.go:4273`) mixes Opus streams into one mixed output
(`ForwardOutput`), and the audio rung runs in both `mixing` and
`selective_forwarding` modes (`audioRungMode`, `main.go:3827`).

Decision: **keep mixing in-process**. The mixer is a mode of the
audio rung, not a separate service.

## Why in-process

- Mixing needs every speaker's decoded frames at the same time; a
  separate mixing service would add a network hop and a second
  failure domain on the audio path for no measured benefit.
- The selective-forwarding mode already exists for rooms that do
  not want a mix (each listener picks speakers); the two modes cover
  both products (mixed room audio vs per-speaker audio) without a
  second deployment.
- The mixer is already measured: the audio rung runs 10 speakers x
  100 listeners in-process in both modes. A separate service would
  need its own benchmark before it could claim to be better.

## What would change the decision

- If mixing CPU becomes the bottleneck for a room (mixing is
  O(speakers) decode + 1 encode per room), the mixer moves out as a
  documented separate service behind the same `ForwardOutput`
  interface, with its own scale gate. That move is a measured
  decision, not a rewrite: the interface is the seam.
- If a room needs a mix in a different codec than the speakers
  publish (transcoding), that is a separate job (transcoding is out
  of scope for 12.1, which is forwarding without transcoding).

## Status

Decision recorded here. The mixer code is in-process and tested by
the audio rung; the decision doc is the deliverable for 12.4.
