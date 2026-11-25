# Canonical vs mirror (job 1.1)

## The two copies

- **ForgeOS copy** (the mirror in the ForgeOS repo):
  `repo-20261125/main-verify.go`, sha256
  `88d619cadaebba6f5bb32df3be2e05838b2cb0fd0169a98b266eebf6cf415d83`.
- **Standalone copy** (this tree's starting point):
  `joblist-tree/main.go`, sha256
  `9b253fafc2a73a251101fd4435c98ec2c6e17b0f07d94594b6d918b2294fd19a`
  (verified first-hand, 2026-11-25).

Full unified diff saved at
`docs/archive/joblist/cluster-a/canonical-vs-mirror.diff` (sha256
`a340627023cafa46ffb9de55ea00690cb64bd1afaaa4e527e5921d57831d761f`).

## The diff, verified first-hand

5 hunks, 26 changed lines (16 added, 10 removed). The first-pass
audit said "38 diff lines"; that count was not reproduced — the
hunk list below is the first-hand count and it matches the audit's
hunk list exactly.

1. **gofmt alignment** (~line 81): `sfuHostMaxUsedSwapMiB = 512`
   spacing. Cosmetic only.
2. **Call-site rename** (~line 1328): `runLocalHardwareProof` →
   `runLocalHardwareProof`. Name only.
3. **Comment wording** (~line 2067): the old comment credited the
   decisive measurement to an external assistant by name; the new
   comment drops the attribution. Comment only. (The verbatim diff
   file below retains the old wording as evidence.)
4. **Audio-rung guard modernised** (~line 4855): the stale
   `swapFree < 2048` guard is replaced by `readSFUHostSnapshot()`
   + `sfuHostMemoryFailures(hostSnapshot, false)`. **This is the
   only substantive drift.** It is a behaviour change in the
   direction the used-swap rule already took (job 18.2): the old
   guard could block a healthy run or pass an unhealthy one
   whenever the swap pool resized.
5. **Function rename + error string** (~line 10577): the
   chip-named proof function → `runLocalHardwareProof`, and its
   error string drops the chip-specific name for the generic
   "hardware-proof duration must be at least …". Name/error text
   only. (The verbatim diff file below retains the old names as
   evidence.)

## Recommendation for the owner (DECISION — no deletion done here)

**Make the standalone repo the canonical home, and treat the
ForgeOS copy as a mirror that is deleted only after the standalone
repo is created and its first sync is verified.**

Reasons: the standalone copy is the one that carries the newer
host-safety guard (hunk 4) and the de-branded names (hunks 2, 3,
5); the ForgeOS copy carries the stale swap guard. Deleting
either copy now would destroy the only record of the drift, so
nothing was deleted in this round.

What the owner still needs to decide: (a) confirm the standalone
repo as canonical; (b) confirm the ForgeOS copy is deleted after
the standalone repo's first verified sync — the actual deletion
and the GitHub repo creation are the owner's actions, not this
tree's.
