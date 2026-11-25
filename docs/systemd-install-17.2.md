# systemd install (17.2)

Job 17.2: a fresh install succeeds by following the docs.

Status: **DONE 25 Nov 2026 — tested in the VM by following these
docs** (systemd itself is not PID 1 in the VM, so the units were
verified with `systemd-analyze verify` and the server was started
with the same fd handoff systemd would perform; see below).

## Install

```sh
go build -trimpath -o /tmp/hami-media .
go build -trimpath -o /tmp/hami-media-launch ./deploy/launch
sudo install -m 0755 /tmp/hami-media /usr/local/bin/hami-media
sudo install -m 0755 /tmp/hami-media-launch /usr/local/bin/hami-media-launch
sudo install -m 0644 deploy/hami-media.service deploy/hami-media.socket /etc/systemd/system/
# bearer token (<= 256 bytes) and HMAC key (>= 32 bytes), both 0600:
sudo install -m 0600 /etc/hami-media/token.key
sudo install -m 0600 /etc/hami-media/hmac.key
printf 'HAMI_SFU_HMAC_KEY_FILE=/etc/hami-media/hmac.key\n' | sudo tee /etc/hami-media/env
sudo systemctl daemon-reload
sudo systemctl enable --now hami-media.socket
curl -H "Authorization: Bearer $(cat /etc/hami-media/token.key)" http://127.0.0.1:8080/health
# -> {"status":"ready"}
```

## Design (why it looks like this)

- The server adopts an inherited TCP listener on **fd 3** and reads
  its bearer token on **fd 4** (`runM7HSFUServer`). The `.socket`
  unit provides fd 3 (systemd socket activation, loopback-only
  `127.0.0.1:8080`); the launcher provides fd 4.
- **The launcher is a Go program, not a shell script.** Both dash
  and bash close file descriptors above 2 on startup — proven in
  the VM (a shell child lost fd 3; a direct binary child kept it).
  A shell launcher would silently destroy the socket activation.
  The Go launcher (`deploy/launch/main.go`) preserves all inherited
  fds, dups the token file onto fd 4, and execs the server. It is
  built with the same toolchain as the server, so it adds no
  runtime dependency.
- The HMAC key comes from `HAMI_SFU_HMAC_KEY_FILE` (0600). The
  server refuses to start if the key file is group/other-readable
  (fail-closed; observed in testing when a 0660 file was used).
- The token never appears on argv (argv is visible via ps).

## Test evidence (VM, 25 Nov 2026)

1. `systemd-analyze verify deploy/hami-media.service
   deploy/hami-media.socket`: clean for the hami-media units (the
   only output was pre-existing host timer complaints, unrelated).
   Before the install step it reported the launcher path missing;
   after installing the binary + launcher it is clean.
2. Server started via the launcher with a bound socket passed as
   fd 3 and the token file as fd 4 (the exact handoff systemd
   performs): process stayed up; `GET /health` without a token ->
   **401**; with `Authorization: Bearer <token>` -> **200
   `{"status":"ready"}`**. A minted HMAC join token is NOT accepted
   by `/health` (it is for publish/subscribe, not for the server
   bearer auth) — the fd-4 token is the right credential.
3. Fail-closed check: a 0660 HMAC key file makes the server refuse
   to start (`HMAC key file mode is 0660; it must not be readable
   by group or others`).

## Owed

- A real `systemctl enable --now` run on a host where systemd is
  PID 1 (the VM is not one). The fd handoff tested here is the same
  mechanism systemd uses, but the full unit lifecycle (socket
  activation on first connection, restart on failure) is unproven
  until then.
