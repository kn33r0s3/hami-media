# Troubleshooting

## "no HMAC key configured"

Set the key. The server refuses to start without it.

```sh
export HAMI_SFU_HMAC_KEY_FILE=/path/to/hmac-key.txt
```

The file must hold at least 32 characters. Do not set both the file and `HAMI_SFU_HMAC_KEY`. Pick one.

## "set only one of HAMI_SFU_HMAC_KEY_FILE and HAMI_SFU_HMAC_KEY"

You set both. Unset one.

## 401 from an endpoint

The Bearer token is missing or wrong. The Bearer token is made fresh per server run and belongs to the harness that started the server. It is not the client token. Use both: Bearer token for the endpoint, client token for the room and role.

## "token expired" or a resume refused

The client token passed its `exp`. Mint a fresh one with `-mode mint-token`. There is no clock-skew allowance.

## A scale run refuses to start

The host health check failed. Read the error: it names the failing check. Common causes:

- Used swap above 512 MiB. The check judges used swap, not free swap (macOS shrinks the pool, so a free-swap floor can never pass). Close other work or wait for the pool to drain.
- No swap pool at all (for example, right after a reboot). Then the check wants 8 GiB of free memory instead — but a zero total that persists after a re-read is treated as unknown and the run refuses to start (fail closed); no-swap protection is unverified in that edge case until the zero-reading glitch is identified (fixed27, REPORT.md §2.8).
- Not enough free disk. The check wants 10 GiB free.
- Memory pressure is high. Wait for other work to finish.

If you are sure the machine can take the run, `-allow-unsafe-host` skips the check. A run that starts unhealthy usually fails loudly, so only skip it when you know why the check failed.

## A run fails after passing before, with no config change

The machine state can change under a run. Four 20 x 8,000 runs
failed catastrophically in one window (~19:29–19:50Z on 24 Nov)
and the identical config passed again afterwards; a 20 x 7,500
run failed on latency in the same window and passed on re-check.
Before changing config, re-run the exact same config once. If it
passes, the failure was transient machine state, not the config.
See REPORT.md §4.5 and §4.8.

## A stall: frames stop but nothing errors

The failing runs show a stall, not a crash: frames unsent while
the scheduler is busy. The gates that catch it are p99 latency
and frame counts — a run where the p50 latency median rises
above ~10 ms (passing runs sit at 1.7–3.7 ms) is stalling even if
p99 has not blown the 150 ms gate yet. A stall gate (fail if p50
median > 10 ms) is proposed, not yet set. See REPORT.md §4.8.

## "bind: address already in use" when repeating a test

The proof tests bind a fixed UDP port per test, and the socket stays bound for the life of the test process. Repeats need separate `go test` processes. Do not use `-count=3` on these tests.

## UDP permission errors on a sandbox or VM

The error looks like a blocked `sendto` with `EPERM`. Some sandboxes block unconnected UDP sends. The unit tests (`go test ./...`) do not need UDP and still pass. The proof modes need real UDP loopback. Run them on a real Mac or Linux machine, not in the sandbox.

## The server process will not start by hand

`-mode m7h-sfu` is started by the harness. It inherits its listening socket on file descriptor 3 and takes its Bearer token from `-m7h-sfu-token` or file descriptor 4. Running it by hand without those will fail with "inherited M7-H loopback listener is missing". That is expected. Use the proof modes (`pair`, `sfu-single`) for hands-on runs.

## A run stops with a heap error

The heap budget was hit. The run stops cleanly instead of growing without limit. For a diagnostic run, lower the audience (`-sfu-publishers`, `-sfu-viewers-per-publisher`) or set `-max-heap-bytes` explicitly and record it as a separate configuration; never change the cap to convert a failed run into a pass. The default budget grows with viewer count only, not resolution or frame rate. It is not a total RSS limit; the 4 GiB Go heap limit applies when `GOMEMLIMIT` is unset.

## `gofmt -l` names a file

Run `gofmt -w` on that file. The repo is `gofmt` clean.

## `go vet` fails

Read the vet output. It names the file and line. Fix it there. Do not skip vet: it is part of the gate.
