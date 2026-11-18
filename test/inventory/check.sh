#!/bin/sh
# Compare the source-derived inventory with coverage.tsv and report gaps.
#   coverage.tsv columns: kind, item, cases (space-separated case ids; empty = not covered)
# Exit 1 when an item has no case, when an item is missing from coverage.tsv,
# or when coverage.tsv lists an item that no longer exists in the code.
# --sync rewrites coverage.tsv: new items are added with no cases, removed items are dropped.
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"
COV="$DIR/coverage.tsv"
INV="$(mktemp)"; trap 'rm -f "$INV" "$INV.new"' EXIT
"$DIR/inventory.sh" | sort -u > "$INV"
[ -f "$COV" ] || : > "$COV"
if [ "${1:-}" = "--sync" ]; then
  awk -F'\t' 'FILENAME==ARGV[1] { c[$1 FS $2] = $3; next } { print $1 FS $2 FS c[$1 FS $2] }' "$COV" "$INV" > "$INV.new"
  mv "$INV.new" "$COV"
fi
awk -F'\t' '
  FILENAME==ARGV[1] { inv[$1 FS $2] = 1; kind[$1]++; next }
  { seen[$1 FS $2] = 1
    if (!(($1 FS $2) in inv)) { stale++; print "STALE   " $1 "\t" $2 }
    else if ($3 != "") covered[$1]++ }
  END {
    for (k in inv) if (!(k in seen)) { missing++; print "MISSING " k }
    printf "\n%-16s %8s %8s\n", "kind", "items", "covered"
    for (k in kind) { printf "%-16s %8d %8d\n", k, kind[k], covered[k]; t += kind[k]; c += covered[k] }
    printf "%-16s %8d %8d\n", "total", t, c
    exit (missing || stale || c < t) ? 1 : 0
  }' "$INV" "$COV"
