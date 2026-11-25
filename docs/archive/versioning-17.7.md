# Versioning and changelog (17.7)

Job 17.7: each tag has a changelog entry.

Status: **DONE (scheme + initial CHANGELOG) 25 Nov 2026.** No tag
has been created yet — the first tag is the release check's
business (17.8), not this file's.

## Scheme

- **Semantic versioning**: `MAJOR.MINOR.PATCH`.
  - MAJOR: incompatible API or wire change.
  - MINOR: new feature, backwards compatible.
  - PATCH: bug fix, no new feature.
- Tags are `vX.Y.Z` (e.g. `v0.1.0`).
- The changelog (`CHANGELOG.md`, Keep-a-Changelog format) is
  updated in the same change as the code, and the tag is created
  only after `scripts/release-check.sh` passes (job 17.8).
- The `[Unreleased]` section collects the next release; at tag
  time it is renamed to the version + date and a fresh
  `[Unreleased]` section is opened.

## First baseline

`v0.1.0` (25 Nov 2026, unreleased as a tag): the tree at `main.go`
sha `9b253faf…` with the measured anchors 20x7,500 (x3) and
20x8,000 (x6 strict). Everything before it is unversioned working
history; the version starts here because the tree is first being
packaged, documented and gated as a releasable product.
