# Third-party license audit (17.5)

Job 17.5: the audit covers Pion and every dependency, and is
committed.

Status: **DONE 25 Nov 2026.**

## Method

`go list -m all` (32 modules incl. the root) on the current tree,
then each module's `LICENSE` file read from the Go module cache
(`go mod download` was run for the eight test-only modules whose
source was not yet cached, so every license below was read from
its actual file, not from memory).

## Table

| Module | Version | License | Notes |
|---|---|---|---|
| github.com/pion/webrtc/v4 | v4.2.26 | MIT | The core dependency (upstream Pion, not the LiveKit warp fork) |
| github.com/pion/ice/v4 | v4.4.8 | MIT | |
| github.com/pion/dtls/v3 | v3.1.15 | MIT | |
| github.com/pion/sctp | v1.12.0 | MIT | |
| github.com/pion/srtp/v3 | v3.1.3 | MIT | |
| github.com/pion/stun/v4 | v4.0.1 | MIT | |
| github.com/pion/turn/v5 | v5.1.3 | MIT | |
| github.com/pion/interceptor | v0.1.51 | MIT | |
| github.com/pion/rtcp | v1.2.20 | MIT | |
| github.com/pion/rtp | v1.10.6 | MIT | |
| github.com/pion/sdp/v3 | v3.0.20 | MIT | |
| github.com/pion/datachannel | v1.6.3 | MIT | |
| github.com/pion/mdns/v2 | v2.2.2 | MIT | |
| github.com/pion/transport/v5 | v5.1.4 | MIT | |
| github.com/pion/logging | v0.2.4 | MIT | |
| github.com/pion/randutil | v0.1.0 | MIT | |
| golang.org/x/crypto | v0.59.0 | BSD 3-Clause | Bumped 25 Nov 2026 (see below) |
| golang.org/x/net | v0.63.0 | BSD 3-Clause | Bumped 25 Nov 2026 (see below) |
| golang.org/x/sys | v0.50.0 | BSD 3-Clause | Bumped with the above |
| golang.org/x/term | v0.48.0 | BSD 3-Clause | |
| golang.org/x/text | v0.44.0 | BSD 3-Clause | |
| golang.org/x/time | v0.14.0 | BSD 3-Clause | |
| github.com/google/uuid | v1.6.0 | BSD 3-Clause | |
| github.com/wlynxg/anet | v0.0.5 | BSD 3-Clause | |
| github.com/davecgh/go-spew | v1.1.1 | ISC | Test-only |
| github.com/onsi/ginkgo | v1.16.5 | MIT | Test-only |
| github.com/onsi/gomega | v1.17.0 | MIT | Test-only |
| github.com/pmezard/go-difflib | v1.0.0 | BSD 3-Clause | Test-only |
| github.com/sclevine/agouti | v3.0.0+incompatible | MIT | Test-only |
| github.com/stretchr/testify | v1.12.1 | MIT | Test-only |
| go.yaml.in/yaml/v3 | v3.0.5 | MIT | |
| gopkg.in/yaml.v3 | v3.0.1 | MIT and Apache (dual) | |

## Findings

- **No copyleft**: no GPL, LGPL, AGPL or MPL anywhere in the tree.
  All licenses are permissive (MIT / BSD 3-Clause / ISC /
  Apache-2.0 dual for yaml.v3).
- **Dependency bump (25 Nov 2026, job 17.1)**: Trivy's filesystem
  scan found 17 HIGH findings in `x/crypto v0.48.0` and `x/net
  v0.50.0` (CVE-2026-39828…56854, CVE-2026-25681…78669), 0
  CRITICAL. Bumped to `x/crypto v0.59.0`, `x/net v0.63.0`,
  `x/sys v0.50.0`; re-scan: **0 HIGH, 0 CRITICAL**. The bump raised
  the `go` directive 1.24.0 -> 1.26.0 (`go mod tidy`, required by
  the fixed deps); the build, vet and new tests pass on Go 1.27.4.
  `go.mod`/`go.sum` changed accordingly — review the diff at merge.
- The NOTICE file in the repo root summarises this table.

## Owed

- Re-run this audit on every dependency change (the release check
  does not yet include a license re-audit; add it when the
  dependency set next changes).
- Codec licensing (H.264) is a separate [!] item (job 6.10, needs
  owner approval) and is NOT covered by this audit.
