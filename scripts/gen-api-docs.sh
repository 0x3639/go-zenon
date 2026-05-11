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
#
# NOTE (v2 rollout): this script depends on AGENTS.md and docs/STYLE.md
# being present at repo root / docs/. Those land in the final v2 PR
# alongside the docs/api/ regeneration. Earlier in the v2 sequence the
# script is checked in but not exercised by CI.

set -euo pipefail

# Force module-mode resolution against this repo's own go.mod, ignoring any
# parent go.work that may chain in unrelated modules. Without this, every Go
# command in the script (including the `go list -m` below) inherits the
# workspace and produces multi-module output that corrupts the generated
# index. Exported once here so all subsequent invocations stay consistent.
export GOWORK=off

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# Preflight: the index writer below pulls package descriptions from AGENTS.md
# and the cross-reference block links to docs/STYLE.md and ARCHITECTURE.md.
# Those files land in the final docs/coverage-v2 PR. Until then, fail fast
# with a targeted message rather than blowing up halfway through with an
# obscure awk "cannot open file" error.
missing=()
for f in AGENTS.md docs/STYLE.md ARCHITECTURE.md; do
  [ -f "$f" ] || missing+=("$f")
done
if [ "${#missing[@]}" -gt 0 ]; then
  cat >&2 <<EOM
gen-api-docs.sh: missing required reference file(s):
  ${missing[*]}

This script needs AGENTS.md (for per-package one-liners), docs/STYLE.md,
and ARCHITECTURE.md (for the index cross-references) at repo root / docs/.
Those files land in the final docs/coverage-v2 PR alongside the docs/api/
regeneration; until that PR merges, \`make doc-api\` is not exercised.

If you need to run this locally before then, grab the missing files from
origin/docs/godoc-coverage with:
  git show origin/docs/godoc-coverage:AGENTS.md > AGENTS.md
  git show origin/docs/godoc-coverage:docs/STYLE.md > docs/STYLE.md
  git show origin/docs/godoc-coverage:ARCHITECTURE.md > ARCHITECTURE.md
EOM
  exit 1
fi

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
    go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@v1.1.0
  fi
fi

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

echo "Generating markdown docs under $OUT_DIR/ ..."

# gomarkdoc writes one .md per package. The {{.Dir}} template expands to the
# package directory relative to the module root, giving us a 1:1 mirror of the
# source tree under docs/api/.
gomarkdoc \
  --output "$OUT_DIR/{{.Dir}}/README.md" \
  --repository.url "https://github.com/zenon-network/go-zenon" \
  --repository.default-branch master \
  ./...

# Second pass for build-tagged packages that the default pass skips.
#
# Build-tag audit (rerun with):
#   grep -rn "^//go:build" --include="*.go" . | grep -v "_test.go" \
#     | awk -F: '{print $3}' | sort -u
#
# Only `libznn` (and its `!libznn` / `libznn && !detached` variants) appears
# as a non-test build tag in this repo. It is present in three directories:
#
#   cmd/libznn/  — the cgo entry point that compiles to libznn.{so,dylib,dll}.
#                  This is unique API surface and is documented by the pass
#                  below.
#   app/         — a libznn-tagged variant of app/manager.go that swaps in
#                  the shared-library lifecycle. The exported API matches the
#                  default !libznn variant already documented by the first pass.
#   metadata/    — same pattern as app/: parallel implementation, identical
#                  exported API as the !libznn build.
#
# We therefore scope this second pass to ./cmd/libznn/... only. Documenting
# the libznn variants of app/ and metadata/ would overwrite the default-pass
# docs for those directories and surface implementation detail rather than
# new API. If a future libznn-tagged package introduces unique exported API,
# either extend this block or move to a per-directory output template.
gomarkdoc \
  --output "$OUT_DIR/{{.Dir}}/README.md" \
  --tags "libznn" \
  --repository.url "https://github.com/zenon-network/go-zenon" \
  --repository.default-branch master \
  ./cmd/libznn/...

# Strip trailing whitespace from generated markdown. gomarkdoc preserves
# trailing spaces and tabs that bleed through from source comments and ABI
# string literals, which trips `git diff --check`. Use sed -i.bak (+ delete)
# for portability between BSD sed (macOS) and GNU sed (Linux CI). Using
# `-exec ... +` rather than `| xargs ...` so the script is a no-op when
# gomarkdoc has produced zero markdown files (which would otherwise vary
# across xargs implementations and `sed -i` with no file arguments).
find "$OUT_DIR" -name '*.md' -exec sed -i.bak 's/[[:space:]]*$//' {} +
find "$OUT_DIR" -name '*.md.bak' -delete

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
