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
  # Find every generated package README and link it.
  find "$OUT_DIR" -mindepth 2 -name 'README.md' \
    | sed "s#^$OUT_DIR/##; s#/README.md\$##" \
    | sort \
    | while read -r pkg; do
        echo "- [\`$MODULE/$pkg\`]($pkg/README.md)"
      done
  echo
  echo "## Cross-references"
  echo
  echo "- [ARCHITECTURE.md](../../ARCHITECTURE.md) — system overview and concept glossary."
  echo "- [AGENTS.md](../../AGENTS.md) — package map and where-to-look index."
  echo "- [STYLE.md](../STYLE.md) — godoc style guide."
} > "$INDEX"

echo "Done. Wrote $(find "$OUT_DIR" -name README.md | wc -l | tr -d ' ') files under $OUT_DIR/."
