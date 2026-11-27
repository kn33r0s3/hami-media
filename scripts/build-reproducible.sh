#!/bin/sh
# Job 1.3 — reproducible build for every command package.
set -eu
ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
OUTDIR="${1:-build/reproducible}"
mkdir -p "$OUTDIR"
for package in "." "./cmd/hami-packager"; do
  name="$(printf '%s' "$package" | sed 's#^\./##; s#/#-#g')"
  [ "$name" = "." ] && name="hami-media"
  [ "$name" = "deploy-launch" ] && name="hami-media-launch"
  CGO_ENABLED=0 go build \
    -trimpath -buildvcs=false -ldflags="-buildid=" \
    -o "$OUTDIR/$name" "$package"
  sha256sum "$OUTDIR/$name"
done
