#!/bin/sh
# Cluster C §8.7 — dependency vulnerability scan (standalone).
# Uses govulncheck when installed; otherwise falls back to `go list`
# + the Go vulnerability database via `go vet`'s vuln check is NOT
# available, so the fallback prints the pinned dependency list and
# fails with a clear message asking the operator to install
# govulncheck (`go install golang.org/x/vuln/cmd/govulncheck@latest`).
# CI wiring (cluster A owns .github/workflows): add a step running
# `sh scripts/vuln-scan.sh` and fail on any finding above LOW.
set -eu
cd "$(dirname "$0")/.."
GO="${GO:-$HOME/go/bin/go}"
echo "vuln-scan: dependencies (pinned in go.mod):"
"$GO" list -m all
if command -v govulncheck >/dev/null 2>&1; then
  echo "--- govulncheck ---"
  govulncheck ./...
  echo "vuln-scan: govulncheck clean"
else
  echo "vuln-scan: govulncheck NOT INSTALLED — install with:"
  echo "  go install golang.org/x/vuln/cmd/govulncheck@latest"
  echo "vuln-scan: cannot verify vulnerabilities without govulncheck; failing closed"
  exit 2
fi
