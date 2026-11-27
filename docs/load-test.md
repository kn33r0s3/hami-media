# Run a local load test

`scripts/load-test.py` is the friendly entry point for an H.264 SFU load
test. It builds the current checkout when needed, supplies the raw input frame
rate, checks the repository's hard caps, preserves a failed run, and writes a
`run.json` manifest with the exact command and links to every log.

## Prerequisites

- Go installed (`go version`).
- A raw Annex-B H.264 video file.
- For a large run, a machine with enough CPU, memory, disk, and working UDP.

Raw H.264 does not contain reliable frame-rate timing metadata. The runner
defaults to 30 fps; set `--frame-rate` to match your file. It does not silently
substitute a generated clip for your input.

If your source is a regular video file rather than raw H.264, convert it once
with FFmpeg. This example creates a 1280×720, 30 fps Annex-B file:

```sh
ffmpeg -i input.mp4 \
  -vf "scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2" \
  -r 30 -an -c:v libx264 -preset veryfast -tune zerolatency \
  -pix_fmt yuv420p -f h264 stream-720p30.h264
```

For an original raw `.h264` input, use that file directly and pass its actual
frame rate; do not convert it merely to satisfy the runner.

## First use

From the repository root, try one publisher and one viewer first:

```sh
python3 scripts/load-test.py --video /path/to/video.h264
```

The binary is built into that run's evidence folder, so a missing or stale
`./hami-media` is not a prerequisite. To reproduce the earlier 10-publisher
test, run:

```sh
python3 scripts/load-test.py \
  --video /path/to/original-1280x720-30fps.h264 \
  --frame-rate 30 \
  --publishers 10 \
  --viewers-per-publisher 500 \
  --duration 60s \
  --yes
```

The `--yes` flag explicitly confirms a run of 1,000 or more simulated viewers.
The host health preflight and heap safety stop remain enabled. Do not bypass
them or raise the heap budget just to make a run pass.

There is a hard limit of 500 viewers per publisher and 10,000 total viewers.
Therefore 10 publishers can represent at most 5,000 viewers; 10,000 simulated
viewers requires at least 20 publishers. These are local simulated peers, not
real devices or proof of internet-scale capacity.

## Evidence and failed attempts

Every invocation gets a unique directory under `results/load-tests/` by
default. `run.json` records the source commit/hash, input hash, configuration,
exact invocation, exit code, and status. `result.json`, `stderr.log`, and
`build.log` are retained even when setup or the run fails. The runner refuses
to overwrite an existing output directory.

Use `--out results/my-run` to choose a directory. Use `--binary ./hami-media`
only when you intentionally want to test a binary you built yourself; the
default always builds from the current source.

Inspect a completed run:

```sh
cat results/load-tests/<run-id>/run.json
cat results/load-tests/<run-id>/result.json
tail -n 80 results/load-tests/<run-id>/stderr.log
```

Do not treat a setup failure (missing input, build failure, or blocked UDP) as
a capacity verdict. Mac and Codespaces results are development evidence, not
the reference-machine proof required by the guidebook.
