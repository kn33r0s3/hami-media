# 12.5 Audio rung benchmark — status

## VM run (this session)

- In-process 1 speaker x 100 listeners (12.1): DONE in-process.
  Test `TestClusterDOpusOneSpeakerHundredListeners` — 50 Opus frames
  forwarded to 100 listeners, payload bytes identical to source
  (no transcoding). Evidence: `log (sha256 efb56b582cc8, 4 lines; kept outside repo)`.
- Real-pair audio rung (`-mode=audio-rung1`, 1 speaker x 100
  listeners, both mixing and selective modes): ATTEMPTED in the VM
  and recorded below. It uses real Pion peer pairs over UDP, so it
  is one of the six known VM sandbox UDP failures — it cannot pass
  in this VM. The command and its output are the evidence, not a
  pass claim.

## Mac run (owed — same gates, reference machine)

Run on the reference machine with the same gates (p99, dissolved
fraction, duration) as the VM attempt:

```sh
cd <hami-media tree>
go run . -mode=audio-rung1 -audio-speakers=1 -audio-listeners=100 -duration=10s
go run . -mode=audio-rung1 -audio-speakers=10 -audio-listeners=100 -duration=10s
```

What is owed from the Mac run: the JSON result files for both
shapes, in both modes (mixing + selective_forwarding), with code
sha, parameters and trial IDs recorded per R4. Until those files
exist, 12.5 stays PARTIAL (VM in-process shape done, Mac real-pair
run owed).

## Why the VM cannot do the real-pair run

The VM sandbox blocks unconnected UDP sends; Pion only ever sends
unconnected UDP. The audio rung's peer pairs therefore cannot
negotiate in the VM. This is the same known block as the six
baseline failures, not a code failure.
