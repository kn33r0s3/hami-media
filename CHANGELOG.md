# Changelog

All notable changes to hami-media. Format: Keep a Changelog
(`## [x.y.z] - YYYY-MM-DD`, `Added` / `Changed` / `Fixed` /
`Security`). Versioning: semantic versioning (`docs/versioning-17.7.md`).
Every tag gets an entry here before the tag is created (job 17.8).

## [Unreleased]

### Added

- Benchmark preregistration (16.1) and win bar (16.2) — frozen before
  the first comparison run (`docs/benchmark-preregistration.json`).
- One benchmark runner with host preflight and R4-schema JSON output
  (`scripts/benchmark-runner.sh`, job 15.5).
- Result schema validator (`scripts/validate-result.py`, job 15.6).
- Counts-table generator from raw files with a REPORT.md §4.5 match
  check (`scripts/generate-counts-table.py`, job 15.7).
- LiveKit comparison generator (`scripts/compare-livekit.py`, job 16.6).
- Hardware record script (`scripts/record-hardware.sh`, job 16.5).
- Uniform host preflight wrapper (`scripts/host-preflight.sh`, job 18.1).
- Deterministic-seed tests (`determinism_test.go`, job 15.2).
- ABORTED-BY-HOST watchdog test and heap-cap test
  (`host_safety_test.go`, jobs 18.4, 18.5).
- Dockerfile (pinned bases, non-root user) + Trivy scans (job 17.1).
- systemd socket + service + Go launcher (job 17.2).
- Architecture, operator, API and SDK docs (job 17.3).
- Known-limitations document (job 17.4).
- Third-party license audit + NOTICE (job 17.5).
- Name and trademark check (job 17.6).
- LiveKit forwarder comparison at v1.13.11 (job 19.1).
- Build/contribute/adopt decision framework (job 19.2).
- Fail-before/pass-after register (job 15.1, partial — most records owed).

### Changed

- Dependency bump: `golang.org/x/crypto` v0.48.0 -> v0.59.0,
  `golang.org/x/net` v0.50.0 -> v0.63.0, `golang.org/x/sys` v0.41.0
  -> v0.50.0; `go` directive 1.24.0 -> 1.26.0 (required by the fixed
  deps). Clears 17 HIGH Trivy findings (0 HIGH, 0 CRITICAL after).

### Fixed

- None in this release (bug fixes in §§2-4 are clusters A/B's work;
  their records land in `docs/fail-before-pass-after-register.md`).

## [0.1.0] - 2026-11-25

Initial versioned baseline. The tree at `main.go` sha
`9b253fafc2a73a251101fd4435c98ec2c6e17b0f07d94594b6d918b2294fd19a`
is the first tagged baseline; everything before it is unversioned
working history. Measured anchors carried into this release:
20x7,500 strict x3, 20x8,000 strict x6 (REPORT.md §4.5).
