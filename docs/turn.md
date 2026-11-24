# TURN

Some viewers sit behind routers that block direct UDP. For them, the server relays video through TURN. Without it, those viewers cannot connect at all.

## Run the TURN server

```sh
export HAMI_SFU_TURN_USERNAME=relay
export HAMI_SFU_TURN_PASSWORD_FILE=/path/to/turn-password.txt
./hami-media -mode turn-server
```

It listens until you stop it. Default listen address is `127.0.0.1:3478`. Change it with `-turn-listen` or `HAMI_SFU_TURN_LISTEN`.

## Settings

| Variable | Default | Meaning |
|---|---|---|
| `HAMI_SFU_TURN_URL` | `turn:127.0.0.1:3478` | The TURN URL clients use. |
| `HAMI_SFU_TURN_USERNAME` | — | TURN username. Required. |
| `HAMI_SFU_TURN_PASSWORD` | — | TURN password. |
| `HAMI_SFU_TURN_PASSWORD_FILE` | — | Path to a file holding the TURN password. Preferred. |
| `HAMI_SFU_TURN_REALM` | `hami` | TURN realm. |
| `HAMI_SFU_TURN_LISTEN` | `127.0.0.1:3478` | Where the server listens. |

Set the password in exactly one place: file or environment, not both. The server refuses to start without a username and a password.

## How clients use it

Clients get the TURN URL as an ICE server in their WebRTC setup. The server hands out the URL from `HAMI_SFU_TURN_URL`. A client behind a strict NAT connects through the relay and receives frames. A client that can connect directly does not need it.

## What is proven, what is not

- The TURN server runs standalone and serves the relay. Its unit tests pass.
- The full gate — a client behind a real NAT, with UDP blocked, connecting through the relay and receiving frames — needs a second machine on a real network. Loopback does not count. That gate is still open. It is not claimed as passed here.
