#!/usr/bin/env bash
# check-comment-only.sh BASE_REF
# Verifies every .go file changed since BASE_REF differs only in comments:
# both versions are parsed, comments dropped, pretty-printed, and compared.
set -euo pipefail
BASE="${1:?usage: check-comment-only.sh BASE_REF}"
cd "$(git rev-parse --show-toplevel)"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir "$TMP/stripper"
cat > "$TMP/stripper/strip.go" <<'EOF'
package main

import (
	"fmt"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
)

// Reads a Go file, drops all comments, prints canonical source to stdout.
func main() {
	src, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, os.Args[1], src, 0) // no ParseComments
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := (&printer.Config{Mode: printer.TabIndent, Tabwidth: 8}).Fprint(os.Stdout, fset, f); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
EOF

STRIP="$TMP/strip"
GOWORK=off go build -o "$STRIP" "$TMP/stripper/strip.go" || { echo "FATAL: could not compile strip helper"; exit 2; }

fail=0
while IFS= read -r f; do
	[[ "$f" == *.go ]] || continue
	git show "$BASE:$f" > "$TMP/old.go" 2>/dev/null || { echo "NEW FILE (not allowed in docs PR): $f"; fail=1; continue; }
	"$STRIP" "$TMP/old.go" > "$TMP/old.stripped" || { echo "PARSE FAIL (base): $f"; fail=1; continue; }
	"$STRIP" "$f"          > "$TMP/new.stripped" || { echo "PARSE FAIL (head): $f"; fail=1; continue; }
	if ! diff -q "$TMP/old.stripped" "$TMP/new.stripped" > /dev/null; then
		echo "NON-COMMENT CHANGE: $f"
		diff "$TMP/old.stripped" "$TMP/new.stripped" | head -10
		fail=1
	fi
done < <(git diff --name-only "$BASE" -- '*.go')

[[ $fail -eq 0 ]] && echo "OK: all changed .go files are comment-only vs $BASE"
exit $fail
