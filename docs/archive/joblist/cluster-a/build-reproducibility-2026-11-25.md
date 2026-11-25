# Build reproducibility (job 1.3) — 2026-11-25

- Tree: `joblist-tree-a`, post-Section-2-fixes state (see the
  source manifest `source-manifest-2026-11-25.sha256` for the exact
  file shas this build came from).
- Command (run twice): `GOTMPDIR=$HOME/.cache/go-tmp
  GOFLAGS=-mod=mod go build -trimpath -o <out> .`
- Toolchain: Go 1.27.4 linux/amd64, module Go 1.24, Pion v4.2.26.
- Build 1: sha256 `1c415003064c556dde85dbc6edc6536181788e634eef2ac961dbc38c0e798cbb`
- Build 2: sha256 `1c415003064c556dde85dbc6edc6536181788e634eef2ac961dbc38c0e798cbb`
- `cmp` confirms the two binaries are byte-identical.
- Size: 20,286,599 bytes each.
- No binary is tracked in the repo: the two build outputs were
  written to /tmp and deleted after comparison.
