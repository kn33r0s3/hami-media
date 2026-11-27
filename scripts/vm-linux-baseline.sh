#!/bin/sh
# Development Linux baseline runner (job 1.4); Docker Linux is not the
# guidebook reference VM. It records the Docker image digest and runs
# the repository test gate from a fresh Linux container.
set -eu
cd "$(dirname "$0")/.."
IMAGE="${HAMI_LINUX_IMAGE:-golang:1.26-bookworm}"
OUTDIR="${1:-results/vm-linux-baseline}"
mkdir -p "$OUTDIR"
docker pull "$IMAGE" >/dev/null
IMAGE_DIGEST="$(docker image inspect --format '{{index .RepoDigests 0}}' "$IMAGE" 2>/dev/null || docker image inspect --format '{{.Id}}' "$IMAGE")"
printf '%s\n' "$IMAGE_DIGEST" > "$OUTDIR/image-digest.txt"
docker run --rm -v "$PWD:/src" -w /src "$IMAGE" sh scripts/test-all.sh > "$OUTDIR/tests.txt" 2>&1
