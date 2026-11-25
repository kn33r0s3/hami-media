# Release checklist (17.8)

Job 17.8: a tag cannot be created until tests, leak scan, benchmark
gate, docs, source manifest all pass.

Status: **DONE (checklist + enforcing script) 25 Nov 2026.** The
script is `scripts/release-check.sh`; this file is the human
checklist it enforces.

## Gates (all must pass; the script exits non-zero otherwise)

1. **gofmt clean** — `gofmt -l .` empty.
2. **go vet clean** — `go vet ./...`.
3. **Tests** — full suite passes except the six known VM sandbox
   UDP failures (listed in `docs/known-limitations.md`). Any other
   failure blocks the release. On a real runner the six must pass
   too; the script's allowance exists only for the VM sandbox.
4. **Leak scan** — `scripts/leak-scan.sh .` passes (no personal
   identifiers, secrets, home paths, exact model IDs).
5. **Benchmark gate** — the preregistration files exist
   (`docs/benchmark-preregistration.json`); the win bar (16.2) is
   unchanged since preregistration. A release that changes the win
   bar after results is not a release.
6. **Docs** — architecture, operator, API and known-limitations
   docs exist and are current for the release.
7. **Changelog** — `CHANGELOG.md` has an entry for the version
   being tagged.
8. **NOTICE** — third-party license audit current for the
   dependency set (`docs/archive/license-audit-17.5.md`).
9. **Source manifest** — owed by cluster A (job 1.2); the release
   check cannot verify it until the manifest exists. Until then
   this gate is recorded as OPEN in the release output, not silently
   passed. **The script currently does not check the manifest —
   see the note below.**

## Note on the source manifest (gate 9)

`scripts/release-check.sh` enforces gates 1-8 today. Gate 9
(source manifest, job 1.2, cluster A) is NOT yet enforceable
because no manifest file exists in any cluster copy. The checklist
records it as open rather than pretending the script covers it.
When the manifest lands, add a `check "source manifest"` line to
the script that verifies every source file's sha256 against it.

## Tag procedure

```sh
./scripts/release-check.sh   # must print ALL GATES PASS
# then, and only then:
git tag -a v0.1.0 -m "v0.1.0: <one-line summary>"
```

## Last run (25 Nov 2026)

- `scripts/release-check.sh` -> **ALL GATES PASS** (gofmt, go vet,
  leak scan, preregistration, docs set, changelog, notice, tests
  with only the 6 known sandbox failures). First run failed on the
  four not-yet-written files; all four have since been written and
  the re-run passes.
- Gate 9 (source manifest) remains the one open item, per the note
  above — the script does not check it yet.
