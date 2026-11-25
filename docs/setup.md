# Setup

## What you need

- Go 1.24 or newer. Check with `go version`.
- A Mac, Linux PC, or Windows PC.
- `ffmpeg` only if you want to make a test video. Any H.264 file works.

## Get the code

```sh
git clone https://github.com/kn33r0s3/hami-media.git
cd hami-media
```

## Build it

```sh
go build -trimpath -o hami-media .
```

The build downloads its dependencies once (Pion WebRTC and friends). After that it works offline.

`-trimpath` matters: without it the binary embeds the builder's home-directory paths. Never ship a binary built without it, and never commit the binary itself (see `.gitignore`).

## Make a test video

Any Annex-B H.264 file works. With `ffmpeg`:

```sh
ffmpeg -f lavfi -i testsrc=duration=3:size=640x360:rate=5 \
  -c:v libx264 -f h264 test.h264
```

## First run

One publisher, one viewer, on your own machine:

```sh
./hami-media -mode pair -video-file test.h264 -duration 3s
```

You should see a short report saying the frames arrived. If it prints an error instead, see [troubleshooting.md](troubleshooting.md).

## Run the tests

```sh
go test ./...
```

This runs the unit tests: about 100 test functions plus 2 benchmarks. They do not need a network. They do not need a video file.

Some tests need UDP loopback. They are the proof modes (`-mode sfu-single` and friends), not `go test`. On a locked-down sandbox, UDP may be blocked and those runs will fail with a permission error. That is the sandbox, not the code. See [troubleshooting.md](troubleshooting.md).

## Check the formatting and the vet

```sh
gofmt -l .
go vet ./...
```

Both should print nothing. If `gofmt -l` names a file, run `gofmt -w` on it.

## What runs where

- macOS: full support. This is where the big runs were measured.
- Linux: full support. Unit tests pass on Linux. UDP loopback tests need a real machine, not a blocked sandbox.
- Windows: builds and runs. The host health checks are simpler there: Windows has no macOS memory-pressure reading, so that one check is skipped.
