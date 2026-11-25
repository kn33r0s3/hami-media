# Install LiveKit OSS as a native binary, pinned version (16.3)

Job 16.3: version and config are recorded, and its own smoke test
passes.

Status: **DONE 25 Nov 2026 (VM install + smoke test).**

## Pinned version

- **LiveKit v1.13.11**, released 2026-10-25T09:45:31Z (latest release
  at the time of install; the pin is this exact version, not
  "latest").
- Tarball:
  `https://github.com/livekit/livekit/releases/download/v1.13.11/livekit_1.13.11_linux_amd64.tar.gz`
  (19,464,037 bytes).
- sha256:
  `1e9ee90ca301e1842b2887fc77cb122759756a9c8aeb4334ee91de3325ec43fb`
  — verified against the release `checksums.txt` (same sha listed
  for `livekit_1.13.11_linux_amd64.tar.gz`).
- Binary reports `livekit-server version 1.13.11`.
- The binary is NOT committed to the repo (no binary is tracked);
  it lives in the VM at `/tmp/livekit-1.13.11/livekit-server`.
  Reinstall from the URL + sha above.

## Config used for the smoke test

`/tmp/lk-smoke/livekit.yaml` (TURN disabled, loopback RTC range
50000-50010, TCP port 7881, log level info), started with:

```
./livekit-server --config /tmp/lk-smoke/livekit.yaml \
  --keys "testkey: testsecret12345678901234567890123456789012"
```

Note: the first config draft used a top-level `bind:` field, which
the server rejected (`could not parse config: yaml: unmarshal
errors: line 2: field bind not found in type config.Config`).
`bind` was removed; the corrected config started cleanly. That
failure and fix are part of the record, not hidden.

## Smoke test result (PASS)

- Server started, logged `starting LiveKit server {"portHttp":
  7880, "nodeID": "ND_ZseosJPMcgW5", "nodeIP": "198.19.0.2",
  "version": "1.13.11", "rtc.portTCP": 7881,
  "rtc.portICERange": [50000, 50010]}`.
- `curl http://127.0.0.1:7880/` returned **HTTP 200, body `OK`**.
- Server shut down cleanly on SIGTERM (`exit requested, shutting
  down`).
- Full log: `log (sha256 786d8358d0a2, 4 lines; kept outside repo)` (copied from the smoke run).

## What this does not prove

The smoke test proves the pinned binary starts and serves HTTP on
this VM. It does not prove LiveKit forwards media here, and it is
not a comparison run — comparison needs the ladder (16.4), which is
BLOCKED on a second machine.
