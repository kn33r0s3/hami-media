#!/usr/bin/env python3
"""Build and run a local H.264 SFU load test with a saved evidence bundle."""

import argparse
import datetime as dt
import hashlib
import json
import math
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import uuid


ROOT = Path(__file__).resolve().parent.parent
DEFAULT_VIDEO = ROOT / "test.h264"
DURATION_RE = re.compile(r"^(?:\d+(?:\.\d+)?(?:ns|us|µs|ms|s|m|h))+$")
HIGH_LOAD_VIEWERS = 1000


def sha256_file(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def utc_now():
    return dt.datetime.now(dt.timezone.utc).isoformat(timespec="seconds")


def parse_args(argv=None):
    parser = argparse.ArgumentParser(
        description="Build hami-media, run a local simulated SFU load test, and save all logs."
    )
    parser.add_argument("--video", default=os.environ.get("VIDEO", str(DEFAULT_VIDEO)),
                        help="raw Annex-B H.264 file (default: $VIDEO or ./test.h264)")
    parser.add_argument("--frame-rate", type=float, default=30,
                        help="input frame rate in fps (default: 30; raw H.264 has no timing metadata)")
    parser.add_argument("--publishers", type=int, default=1,
                        help="simulated publishers (default: 1)")
    parser.add_argument("--viewers-per-publisher", type=int, default=1,
                        help="simulated viewers per publisher (default: 1, hard limit: 500)")
    parser.add_argument("--duration", default="10s",
                        help="media send duration (Go duration, default: 10s)")
    parser.add_argument("--out", type=Path,
                        help="evidence directory (default: results/load-tests/<UTC run id>)")
    parser.add_argument("--binary", type=Path,
                        help="use an existing binary instead of building one")
    parser.add_argument("--yes", action="store_true",
                        help=f"confirm runs with {HIGH_LOAD_VIEWERS}+ simulated viewers")
    return parser.parse_args(argv)


def validate(args):
    errors = []
    video = Path(args.video).expanduser().resolve()
    if not video.is_file() or video.stat().st_size == 0:
        errors.append(f"video file does not exist or is empty: {video}")
    if not 1 <= args.publishers <= 3000:
        errors.append("--publishers must be between 1 and 3000")
    if not 1 <= args.viewers_per_publisher <= 500:
        errors.append("--viewers-per-publisher must be between 1 and 500")
    viewers = args.publishers * args.viewers_per_publisher
    if viewers > 10_000:
        errors.append(f"requested {viewers:,} viewers exceeds the hard limit of 10,000")
    if not math.isfinite(args.frame_rate) or not 1 <= args.frame_rate <= 120:
        errors.append("--frame-rate must be between 1 and 120 fps")
    if not DURATION_RE.fullmatch(args.duration):
        errors.append("--duration must be a positive Go duration such as 10s, 1m, or 1h")
    else:
        units_in_seconds = {
            "ns": 1e-9, "us": 1e-6, "µs": 1e-6, "ms": 1e-3,
            "s": 1, "m": 60, "h": 3600,
        }
        duration_seconds = sum(
            float(amount) * units_in_seconds[unit]
            for amount, unit in re.findall(r"(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)", args.duration)
        )
        if duration_seconds <= 0:
            errors.append("--duration must be greater than zero")
    if viewers >= HIGH_LOAD_VIEWERS and not args.yes and not errors:
        errors.append(
            f"{viewers:,} simulated viewers is a high-load run; re-run with --yes after "
            "checking host capacity"
        )
    return video, viewers, errors


def write_manifest(path, doc):
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(doc, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(path)


def main(argv=None):
    args = parse_args(argv)
    video, viewers, errors = validate(args)
    run_id = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-" + uuid.uuid4().hex[:6]
    out = (args.out or (Path("results/load-tests") / run_id)).expanduser().resolve()
    if out.exists():
        print(f"load-test: evidence directory already exists; refusing to overwrite: {out}", file=sys.stderr)
        return 2
    out.mkdir(parents=True)

    stdout_path = out / "result.json"
    stderr_path = out / "stderr.log"
    build_path = out / "build.log"
    stdout_path.touch()
    stderr_path.touch()
    build_path.touch()
    started = utc_now()
    source_sha = sha256_file(ROOT / "main.go")
    video_sha = sha256_file(video) if video.is_file() else None
    command = []
    doc = {
        "schema": "hami-media-load-test/v1",
        "trial_id": run_id,
        "trial_ids": [run_id],
        "started_at_utc": started,
        "finished_at_utc": None,
        "status": "setup-failed",
        "exit_code": None,
        "code_sha256_main_go": source_sha,
        "runner_sha256": sha256_file(Path(__file__).resolve()),
        "source_commit": None,
        "image_hash": "n/a-native",
        "video": str(video),
        "video_sha256": video_sha,
        "parameters": {
            "publishers": args.publishers,
            "viewers_per_publisher": args.viewers_per_publisher,
            "total_simulated_viewers": viewers,
            "frame_rate_fps": args.frame_rate,
            "duration": args.duration,
        },
        "command": command,
        "raw_logs": ["result.json", "stderr.log", "build.log"],
    }

    try:
        commit = subprocess.run(
            ["git", "-C", str(ROOT), "rev-parse", "HEAD"],
            check=True, capture_output=True, text=True,
        ).stdout.strip()
        doc["source_commit"] = commit
    except (OSError, subprocess.CalledProcessError):
        doc["source_commit"] = "unavailable"

    try:
        if errors:
            raise ValueError("\n".join(errors))
        if args.binary:
            binary = args.binary.expanduser().resolve()
            if not binary.is_file() or not os.access(binary, os.X_OK):
                raise ValueError(f"--binary is not an executable file: {binary}")
        else:
            go = os.environ.get("GO", "go")
            build_command = [go, "build", "-trimpath", "-o", str(out / "hami-media"), "."]
            with build_path.open("w", encoding="utf-8") as build_log:
                subprocess.run(build_command, cwd=ROOT, stdout=build_log, stderr=subprocess.STDOUT, check=True)
            binary = out / "hami-media"
        doc["binary_sha256"] = sha256_file(binary)

        command = [
            str(binary), "-mode", "sfu-single",
            "-video-file", str(video), "-frame-rate", str(args.frame_rate),
            "-sfu-publishers", str(args.publishers),
            "-sfu-viewers-per-publisher", str(args.viewers_per_publisher),
            "-duration", args.duration,
        ]
        doc["command"] = command
        print(f"Load: {args.publishers} publisher(s) × {args.viewers_per_publisher} viewer(s) "
              f"= {viewers:,} simulated viewers")
        print(f"Input: {video} ({args.frame_rate:g} fps)")
        print(f"Evidence: {out}")
        if viewers >= HIGH_LOAD_VIEWERS:
            print("High-load run confirmed. Host preflight and heap safety limits remain enabled.")
        print("The built-in host safety check is enabled; an unhealthy host may delay or stop this run.")
        with stdout_path.open("w", encoding="utf-8") as stdout, stderr_path.open("w", encoding="utf-8") as stderr:
            result = subprocess.run(command, cwd=ROOT, stdout=stdout, stderr=stderr, check=False)
        doc["process_exit_code"] = result.returncode
        return_code = result.returncode if result.returncode >= 0 else 128 + abs(result.returncode)
        doc["exit_code"] = return_code
        doc["status"] = "pass" if result.returncode == 0 else "fail-or-safety-stopped"
    except (OSError, subprocess.CalledProcessError, ValueError) as error:
        message = str(error)
        with stderr_path.open("a", encoding="utf-8") as stderr:
            stderr.write(f"load-test setup error: {message}\n")
        print(f"load-test: {message}", file=sys.stderr)
        if not video.is_file():
            print("Tip: provide the original raw H.264 file with --video /path/to/video.h264 "
                  "(or set VIDEO=/path/to/video.h264).", file=sys.stderr)
        if not args.binary and not errors and isinstance(error, (OSError, subprocess.CalledProcessError)):
            print("Tip: confirm Go is installed with `go version`.", file=sys.stderr)
        return_code = 2
        doc["exit_code"] = return_code
    except KeyboardInterrupt:
        with stderr_path.open("a", encoding="utf-8") as stderr:
            stderr.write("load-test interrupted by operator (Ctrl-C)\n")
        return_code = 130
        doc["exit_code"] = return_code
        doc["status"] = "interrupted"

    doc["finished_at_utc"] = utc_now()
    write_manifest(out / "run.json", doc)
    print(f"Run status: {doc['status']} (exit {return_code})")
    print(f"Evidence saved: {out}")
    print(f"Inspect with: cat {shlex.quote(str(out / 'run.json'))}; "
          f"tail -n 40 {shlex.quote(str(stderr_path))}; "
          f"tail -n 40 {shlex.quote(str(build_path))}")
    return return_code


if __name__ == "__main__":
    raise SystemExit(main())
