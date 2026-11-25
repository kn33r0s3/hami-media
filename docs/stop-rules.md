# Stop rules for each phase (job 0.7)

A stop is reported with raw evidence (R4: code sha, parameters,
trial IDs, raw logs). A stopped phase does not proceed to the next
phase until the stop condition clears or the owner decides
otherwise. "Stop" means stop that phase's work, not stop all work:
other phases continue unless their own rule fires.

| Phase (job-list section) | Stop condition — stop if … |
|---|---|
| 0 Governance | A change is proposed for sync with no passing VM run in the change log, or a leak scan fails. |
| 1 Repo, environment, baseline | The VM baseline grows a failure that is not one of the known sandbox UDP blocks, or two builds of one tree give different binaries. |
| 2 Known correctness bugs | A fix has no failing test first, or its pass-after run fails, or a fix changes a gate result on the reference machine. |
| 3 Media timing | The timestamp fix moves p99 latency the wrong way in the before/after run, or wraparound tests fail. |
| 4 Loss recovery, deadlines, slow viewers | A forced-drop run shows a drop that is not accounted for (no retransmit and no keyframe recovery), or a missing frame blocks later frames. |
| 5 Forwarding core | The mutex profile shows the router lock as the top contention entry with no fix in hand, or goroutine/RSS figures cannot be reconciled with a fresh same-binary run. |
| 6 Codecs, layers, adaptation | A layer switch shows a gap longer than one GOP, or a codec is neither tested end to end nor listed as unsupported. |
| 7 Network | No client connects through the impairment matrix at 3% loss / 150 ms delay, or TURN cannot be reached from the simulated symmetric NAT. |
| 8 Security and access | Any forged, expired, or wrong-room token is accepted, or a secret appears in the repo. |
| 9 Signaling, rooms, API | A room's participants leak into another room, or a reconnect loses state inside the agreed window. |
| 10 Clients | A platform in the device matrix cannot play the top layer and does not fall back cleanly. |
| 11 Multi-node | A killed node's rooms do not reconnect elsewhere within the agreed time. |
| 12 Audio | One speaker to 100 listeners loses audio frames end to end, or lip-sync exceeds the stated tolerance. |
| 13 Outputs | A segment does not start on a keyframe, or a recording round trip does not play back. |
| 14 Observability and operations | A shutdown test ends with leaked goroutines, or a failing dependency still reports ready. |
| 15 Testing, CI, benchmarks | CI fails, or a benchmark run is missing its sha, parameters, trial IDs, or raw logs. |
| 16 Head-to-head and decision | Decision gate A fires: hami-media misses the preregistered win bar at matched load — product work stops and LiveKit OSS is the platform (job 16.7). The win bar itself is never changed after results come in (R6). |
| 17 Packaging, docs, licensing, release | The image scan has a critical finding, or a tag is attempted before tests, leak scan, benchmark gate, docs, and source manifest all pass. |
| 18 Host safety and reference machine | The preflight refuses the run three times in a row on a healthy machine, or a forced host violation is logged as a capacity failure instead of ABORTED-BY-HOST. |
| 19 Upstream and strategy | Decision gate B cannot be run (no second hardware class / real network) — no claim is made before those tests exist. |

Recorded: 2026-11-25, cluster A. Each rule is a starting rule;
the owner may tighten any of them. Loosening a rule needs the
owner's word and a dated note here.
