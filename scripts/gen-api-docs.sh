#!/usr/bin/env bash
# gen-api-docs.sh — Render every package's godoc as markdown under docs/api/.
#
# Output is checked into the repo so reviewers and AI agents can read the
# rendered API surface directly on GitHub without a pkg.go.dev round-trip and
# without running godoc locally. Regenerate any time package comments change:
#
#   make doc-api
#
# This script does not modify go.mod. gomarkdoc is installed into GOBIN
# on demand so the project's own dependency graph stays clean.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

OUT_DIR="docs/api"
MODULE="$(go list -m)"

if ! command -v gomarkdoc >/dev/null 2>&1; then
  GOBIN="$(go env GOBIN)"
  if [ -z "$GOBIN" ]; then
    GOBIN="$(go env GOPATH)/bin"
  fi
  export PATH="$GOBIN:$PATH"
  if ! command -v gomarkdoc >/dev/null 2>&1; then
    echo "Installing gomarkdoc into $GOBIN..."
    GOWORK=off go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@v1.1.0
  fi
fi

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

echo "Generating markdown docs under $OUT_DIR/ ..."

# gomarkdoc writes one .md per package. The {{.Dir}} template expands to the
# package directory relative to the module root, giving us a 1:1 mirror of the
# source tree under docs/api/.
GOWORK=off gomarkdoc \
  --output "$OUT_DIR/{{.Dir}}/README.md" \
  --include-unexported \
  --repository.url "https://github.com/zenon-network/go-zenon" \
  --repository.default-branch master \
  ./...

# Second pass for build-tagged packages that the default pass skips. cmd/libznn
# is gated by `//go:build libznn && !detached` and would otherwise produce no
# documentation under the default build constraints.
GOWORK=off gomarkdoc \
  --output "$OUT_DIR/{{.Dir}}/README.md" \
  --include-unexported \
  --tags "libznn" \
  --repository.url "https://github.com/zenon-network/go-zenon" \
  --repository.default-branch master \
  ./cmd/libznn/...

# Build a TSV lookup of `package path -> one-liner description` from the
# package-map table in AGENTS.md. AGENTS.md is the single source of truth for
# these summaries — we strip the markdown table syntax and the trailing slash
# so the keys match the directory paths emitted by gomarkdoc. Packages absent
# from AGENTS.md fall through to a bare link with no summary, so the index
# stays valid even if AGENTS.md drifts.
SUMMARIES_TSV="$(mktemp)"
trap 'rm -f "$SUMMARIES_TSV"' EXIT
awk -F'|' '
  /^\| `[^`]+\/` \| / {
    path = $2; desc = $3
    gsub(/^[ ]+|[ ]+$/, "", path)
    gsub(/^`|`$/, "", path)
    sub(/\/$/, "", path)
    gsub(/^[ ]+|[ ]+$/, "", desc)
    print path "\t" desc
  }
' AGENTS.md > "$SUMMARIES_TSV"

# Build the top-level index.
INDEX="$OUT_DIR/README.md"
{
  echo "# go-zenon API documentation"
  echo
  echo "Auto-generated rendering of every package's godoc, refreshed by \`make doc-api\`."
  echo "The canonical source of truth is the godoc comments in the Go source — this"
  echo "directory is a convenience for GitHub-side review. For local interactive"
  echo "browsing run \`make docs\` (godoc on :6060). After upstream merge the same"
  echo "content publishes to pkg.go.dev under the canonical module path."
  echo
  echo "Module: \`$MODULE\`"
  echo
  echo "## Packages"
  echo
  # Find every generated package README and link it, joining each link with
  # the AGENTS.md one-liner via the lookup file built above.
  find "$OUT_DIR" -mindepth 2 -name 'README.md' \
    | sed "s#^$OUT_DIR/##; s#/README.md\$##" \
    | sort \
    | awk -F'\t' -v module="$MODULE" '
        FNR == NR { desc[$1] = $2; next }
        {
          link = "- [`" module "/" $0 "`](" $0 "/README.md)"
          if ($0 in desc) print link " — " desc[$0]
          else print link
        }
      ' "$SUMMARIES_TSV" -
  echo
  echo "## Cross-references"
  echo
  echo "- [ARCHITECTURE.md](../../ARCHITECTURE.md) — system overview and concept glossary."
  echo "- [AGENTS.md](../../AGENTS.md) — package map and where-to-look index."
  echo "- [STYLE.md](../STYLE.md) — godoc style guide."
} > "$INDEX"

echo "Done. Wrote $(find "$OUT_DIR" -name README.md | wc -l | tr -d ' ') files under $OUT_DIR/."
