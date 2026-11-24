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

- Not enough free swap. The check wants 2 GiB free. Close other work or add swap.
- No swap pool at all (for example, right after a reboot). Then the check wants 8 GiB of free memory instead.
- Not enough free disk. The check wants 10 GiB free.
- Memory pressure is high. Wait for other work to finish.

If you are sure the machine can take the run, `-allow-unsafe-host` skips the check. A run that starts unhealthy usually fails loudly, so only skip it when you know why the check failed.

## "bind: address already in use" when repeating a test

The proof tests bind a fixed UDP port per test, and the socket stays bound for the life of the test process. Repeats need separate `go test` processes. Do not use `-count=3` on these tests.

## UDP permission errors on a sandbox or VM

The error looks like a blocked `sendto` with `EPERM`. Some sandboxes block unconnected UDP sends. The unit tests (`go test ./...`) do not need UDP and still pass. The proof modes need real UDP loopback. Run them on a real Mac or Linux machine, not in the sandbox.

## The server process will not start by hand

`-mode m7h-sfu` is started by the harness. It inherits its listening socket on file descriptor 3 and takes its Bearer token from `-m7h-sfu-token` or file descriptor 4. Running it by hand without those will fail with "inherited M7-H loopback listener is missing". That is expected. Use the proof modes (`pair`, `sfu-single`) for hands-on runs.

## A run stops with a heap error

The heap budget was hit. The run stops cleanly instead of growing without limit. Either lower the audience (`-sfu-publishers`, `-sfu-viewers-per-publisher`) or raise the budget with `-max-heap-bytes`. The default budget grows with the viewer count; the 4 GiB Go heap limit applies when `GOMEMLIMIT` is unset.

## `gofmt -l` names a file

Run `gofmt -w` on that file. The repo is `gofmt` clean.

## `go vet` fails

Read the vet output. It names the file and line. Fix it there. Do not skip vet: it is part of the gate.
