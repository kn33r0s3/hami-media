#!/bin/sh
# One test command (job 1.8): unit + race + integration + fuzz smoke,
# then one summary. Run from the repo root:
#   sh scripts/test-all.sh
# The repo is a single Go package (main); "integration" tests are the
# local-pair / SFU / TURN tests inside the same suite, not a separate
# package. There are currently zero `func Fuzz` targets (job 8.6 is
# OPEN), so the fuzz step reports that and continues instead of
# failing.
#
# Known VM limit: this sandbox blocks unconnected UDP, so exactly
# six tests fail here for that reason, not code. The summary lists
# them separately. On a machine where UDP works they must pass;
# the script exits non-zero whenever any test fails.
#
# NACK carve-out (Option A, docs/nack-test-carve-out.md):
# TestLocalSFURoutesAuthorizedStreamsAndRTCP is skipped in the
# default suite and run separately below with
# -tags hami_disable_srtp_replay. The test counts duplicate-sequence
# packets at the viewer, but job 2.1 restored SRTP replay protection,
# so the viewer's SRTP layer drops the re-sent copy as a replay and
# the gate can never pass on a default build. The tag invocation must
# -run ONLY that test (never the whole suite: the replay test fails
# under the tag by design) and the tag must never be used for a
# shipped/production binary. In this VM the tag changes nothing for
# this test (it still fails at ICE), so it stays in KNOWN below.
set -u
cd "$(dirname "$0")/.."

# The race build writes large work files; /tmp here is a 512 MiB
# tmpfs and runs out of space. Use the home volume unless the
# caller already set GOTMPDIR.
GOTMPDIR="${GOTMPDIR:-$HOME/.cache/go-tmp}"
mkdir -p "$GOTMPDIR"
export GOTMPDIR
export GOFLAGS="${GOFLAGS:--mod=mod}"
export PATH="$HOME/go/bin:$PATH"

KNOWN="TestLocalPairReceivesAudioAndVideoRTP TestCountingUDPMuxCountsPeerWritesWithoutClosingSharedMux TestPacketReadCountingConnCountsDatagrams TestLocalSFURoutesAuthorizedStreamsAndRTCP TestLocalSFUCascadeRoutesRTCPAcrossWorkers TestSFUTURNRelayCarriesMedia"

TMPDIR_WORK="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_WORK"' EXIT INT TERM

overall=0

echo "== leak scan =="
if python3 scripts/leak-scan.py . > "$TMPDIR_WORK/leak.log" 2>&1; then
  echo "leak scan: PASS"
else
  echo "leak scan: FAIL (see $TMPDIR_WORK/leak.log)"
  cat "$TMPDIR_WORK/leak.log"
  overall=1
fi

echo "== gofmt =="
if [ -z "$(gofmt -l .)" ]; then
  echo "gofmt: PASS"
else
  echo "gofmt: FAIL (unformatted: $(gofmt -l . | tr '\n' ' '))"
  overall=1
fi

echo "== go vet =="
if go vet ./... > "$TMPDIR_WORK/vet.log" 2>&1; then
  echo "go vet: PASS"
else
  echo "go vet: FAIL (see $TMPDIR_WORK/vet.log)"
  cat "$TMPDIR_WORK/vet.log"
  overall=1
fi

echo "== unit + integration (go test) =="
# The NACK loss test is skipped here; it runs separately below with
# the carve-out tag (docs/nack-test-carve-out.md).
if go test -skip 'TestLocalSFURoutesAuthorizedStreamsAndRTCP' -count=1 ./... > "$TMPDIR_WORK/unit.log" 2>&1; then
  echo "unit: PASS"
else
  echo "unit: FAIL"
  grep "^--- FAIL" "$TMPDIR_WORK/unit.log" || true
  overall=1
fi

echo "== NACK loss test (tagged carve-out, see docs/nack-test-carve-out.md) =="
# -run ONLY this test, never the whole suite: the replay test fails
# under this tag by design. The tag is test-only, never for a
# shipped/production binary.
if go test -tags hami_disable_srtp_replay -run 'TestLocalSFURoutesAuthorizedStreamsAndRTCP' -count=1 ./... > "$TMPDIR_WORK/nack.log" 2>&1; then
  echo "nack loss test: PASS"
else
  echo "nack loss test: FAIL"
  grep "^--- FAIL" "$TMPDIR_WORK/nack.log" || true
  overall=1
fi

echo "== race (go test -race) =="
if go test -race -skip 'TestLocalSFURoutesAuthorizedStreamsAndRTCP' -count=1 ./... > "$TMPDIR_WORK/race.log" 2>&1; then
  echo "race: PASS (no data races)"
else
  echo "race: FAIL"
  grep -E "DATA RACE|^--- FAIL" "$TMPDIR_WORK/race.log" | head -n 20 || true
  overall=1
fi

echo "== fuzz smoke =="
if grep -rq "^func Fuzz" --include="*_test.go" .; then
  # One short fuzz run per target; a target that finds a crash fails.
  if go test -run="^Fuzz" -fuzz="Fuzz" -fuzztime=10s ./... > "$TMPDIR_WORK/fuzz.log" 2>&1; then
    echo "fuzz smoke: PASS"
  else
    echo "fuzz smoke: FAIL (see $TMPDIR_WORK/fuzz.log)"
    overall=1
  fi
else
  echo "fuzz smoke: SKIPPED — no func Fuzz targets in the tree (job 8.6 OPEN)"
fi

echo "== summary =="
if [ "$overall" -eq 0 ]; then
  echo "test-all: ALL PASS (leak, gofmt, vet, unit, nack, race, fuzz)"
else
  fails="$(grep "^--- FAIL" "$TMPDIR_WORK/unit.log" "$TMPDIR_WORK/race.log" "$TMPDIR_WORK/nack.log" 2>/dev/null | sed 's/.*FAIL: \([A-Za-z0-9]*\).*/\1/' | sort -u | tr '\n' ' ')"
  known_only=1
  for f in $fails; do
    case " $KNOWN " in *" $f "*) ;; *) known_only=0 ;; esac
  done
  if [ -n "$fails" ] && [ "$known_only" -eq 1 ]; then
    echo "test-all: FAIL — only the known sandbox UDP failures: $fails"
    echo "  (VM sandbox blocks unconnected UDP; these must pass on a machine where UDP works)"
  elif [ -n "$fails" ]; then
    echo "test-all: FAIL — failing tests: $fails"
  else
    echo "test-all: FAIL — see the sections above"
  fi
  echo "per-step logs were in a temp dir that is cleaned up on exit; re-run a step directly to inspect it"
fi
exit "$overall"
