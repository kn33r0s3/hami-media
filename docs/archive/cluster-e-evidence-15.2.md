# Deterministic seeds — evidence (15.2)

Job 15.2: repeated runs of seeded tests give identical results.

Status: **DONE 25 Nov 2026** (for the seeded surface that exists).

## What was verified

The tree has no seeded randomness in production code (grep for
`math/rand` across `*.go`: no hits). Determinism therefore lives in
the test inputs, and the tests in `determinism_test.go` pin it:

- `TestSeededPacketOrderIsIdenticalAcrossRuns` — the same seed
  (20261125) gives the same shuffled packet order; a different seed
  gives a different order.
- `TestSeededAssemblerGivesIdenticalResults` — the same seeded
  packet order fed to the frame assembler gives the same completed
  frames, `[10 20 30]`, twice. The assembler is primed with the known
  first sequence (100): without priming it deliberately refuses to
  record the first frame it sees (no known boundary), and a shuffled
  arrival order would lose frames for that reason, not for
  nondeterminism. That behavior was found by this test's first run
  (it returned `[30]` unprimed) and is documented in the test.
- `TestSeededLossPatternIsIdenticalAcrossRuns` — a 5% loss set
  derived only from (seed, sequence range) is identical on every
  run, and differs for a different seed. This is the shape the future
  deterministic fault-injection suite (4.6) must have.

## Run log

```
$ go test -run 'TestSeeded' -count=2 -v .
--- PASS: TestSeededPacketOrderIsIdenticalAcrossRuns (0.00s)
--- PASS: TestSeededAssemblerGivesIdenticalResults (0.00s)
--- PASS: TestSeededLossPatternIsIdenticalAcrossRuns (0.00s)
--- PASS: TestSeededPacketOrderIsIdenticalAcrossRuns (0.00s)
--- PASS: TestSeededAssemblerGivesIdenticalResults (0.00s)
--- PASS: TestSeededLossPatternIsIdenticalAcrossRuns (0.00s)
PASS
```

Code sha: `main.go`
`9b253fafc2a73a251101fd4435c98ec2c6e17b0f07d94594b6d918b2294fd19a`.

## Rule going forward

If a seeded path is ever added to production code (fault injection,
jitter, loss), it must take its seed from an explicit parameter and
gain a test of this shape: same seed in, byte-identical result out.
