# hami-media

> hami-media is a small video server. One person streams live video. Thousands of people watch it at the same time, in one room. It runs on your own computer. No cloud service is needed.

## What it does

- One room, many viewers. A publisher sends one video stream. The server copies it to every viewer.
- It speaks WebRTC. Viewers watch in a normal browser. No plug-in is needed.
- It recovers lost frames. If a video packet is lost on the way, the server asks for it again.
- It checks who you are. Publishers and viewers need a short-lived token. No token, no stream.
- It works behind a home router too. A built-in TURN server relays video when a direct connection is blocked.
- It measures itself. Every run reports how many frames arrived and how late they were.

## Run it in 5 minutes

You need Go 1.24 or newer. macOS, Linux, and Windows all work.

```sh
git clone https://github.com/kn33r0s3/hami-media.git
cd hami-media
go build -o hami-media .
```

Make a short test video (needs `ffmpeg`, any small clip works):

```sh
ffmpeg -f lavfi -i testsrc=duration=3:size=640x360:rate=5 \
  -c:v libx264 -f h264 test.h264
```

Run one publisher and one viewer on your own machine:

```sh
./hami-media -mode pair -video-file test.h264 -duration 3s
```

If it works, you will see a short report that says the frames arrived. That is the whole proof: video went in one side and came out the other.

Run the tests:

```sh
go test ./...
```

## The auto capacity detector

hami-media looks at your machine before it runs a big stream. It reads memory pressure, free swap, free memory, and free disk. If the machine is not healthy enough, it refuses to start the run instead of failing halfway through. While a run is going, a watchdog keeps watching and stops the run if the machine runs out of room.

At server startup the detector also sets the limits for you. It takes the biggest audience ever measured (20 publishers, 7,500 viewers) and scales it down for weaker machines by one rule: the smaller of cores/10 and RAM GiB/24. A stronger machine never gets more than the measured maximum. See what your machine gets with `./hami-media capacity`. You can always override it. See [docs/capacity.md](docs/capacity.md).

## How to override limits

Flags beat the detector. The main ones:

```sh
# 20 publishers, 500 viewers each (10,000 viewers total)
./hami-media -mode sfu-single -video-file test.h264 \
  -sfu-publishers 20 -sfu-viewers-per-publisher 500 -duration 65s

# Skip the host health check (only if you know what you are doing)
./hami-media -mode sfu-single -video-file test.h264 \
  -sfu-publishers 20 -sfu-viewers-per-publisher 500 \
  -allow-unsafe-host -duration 65s

# Cap the heap; the run stops instead of growing without limit
./hami-media -mode sfu-single -video-file test.h264 \
  -sfu-publishers 20 -sfu-viewers-per-publisher 500 \
  -max-heap-bytes 4000000000 -duration 65s
```

Hard caps in the code: 500 viewers per publisher, 10,000 viewers total, and a delivered-frame p99 latency of 150 ms. See [docs/configuration.md](docs/configuration.md) for every flag and setting.

## Use it in your own code

Today hami-media is a program you run, not a library you import. That is deliberate for this first release: the server core still lives in one `package main` (see [Known limits](#known-limits)). You use it in three ways:

1. **Run the binary.** Build it and run the modes above. This is the supported path.
2. **Talk to its HTTP API.** The server has endpoints for health, publish, view, stats, and shutdown. They all need a Bearer token plus a per-client token. See [docs/api-auth.md](docs/api-auth.md).
3. **Run its TURN server.** `./hami-media -mode turn-server` relays video for viewers behind strict routers. See [docs/turn.md](docs/turn.md).

Splitting the server core into an importable Go package is planned work. It is listed under [Known limits](#known-limits).

## What it is built on

- Go, standard library first.
- [Pion WebRTC](https://github.com/pion/webrtc) v4.2.26 for the media path.
- No cloud service. No account. Your machine is the server.

## Docs

- [Setup](docs/setup.md) — install, build, first run.
- [Configuration](docs/configuration.md) — every flag and setting.
- [Capacity](docs/capacity.md) — measured limits, and the detector.
- [API and auth](docs/api-auth.md) — endpoints and tokens.
- [TURN](docs/turn.md) — relaying for viewers behind strict routers.
- [Troubleshooting](docs/troubleshooting.md) — common failures.
- [Measurement harness notes](docs/integration.md) — how latency is measured.
- [Auth and TURN design notes](docs/auth-turn.md) — why auth works this way.

## Known limits

- **No library package yet.** The server core is one `package main`. A split into an importable package with a thin command is the next structural job. It was left out of this release on purpose: a second change to the same files is landing now, and the split must be done against the finished files, not a snapshot.
- **The detector only knows what was measured.** Its recommendation comes from the anchor table (20 x 5,000 and 20 x 7,500 passed; 20 x 10,000 failed at the Mac loopback wall). A pending 10 x 6,000 row is not used until its measurement lands. See [docs/capacity.md](docs/capacity.md).
- **One room per server process.** That is the design, not a bug.
- **10,000 viewers is a measured wall on one Mac.** At 20 publishers x 10,000 viewers the macOS loopback path itself runs out, not this code. See [docs/capacity.md](docs/capacity.md).

## Where to get help

Open an issue at <https://github.com/kn33r0s3/hami-media/issues>. Say what you ran, what you expected, and paste the last lines of output. If a run failed, include the full report line: it prints the numbers that matter.

## License

Apache 2.0. See [LICENSE](LICENSE). You can use this code in your own project, for free, including in commercial work. Keep the license file with it.
