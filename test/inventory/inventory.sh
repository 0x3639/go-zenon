#!/bin/sh
# Print every testable item of go-zenon, one per line, as "kind<TAB>item".
# The list is derived from the source, so anything added to the code shows up here
# and, until a test case is recorded for it in coverage.tsv, in check.sh's gap report.
set -eu
cd "$(dirname "$0")/../.."
src() { find "$@" -name '*.go' ! -name '*_test.go' | sort; }

# Embedded contract methods: ABI function entries, by definition file.
for f in $(src vm/embedded/definition); do
  grep -o '{"type":"function","name":"[A-Za-z0-9]*"' "$f" 2>/dev/null |
    sed -E "s|.*\"name\":\"([A-Za-z0-9]*)\"|contract-method\t$(basename "$f" .go).\1|" || true
done

# RPC methods: exported methods on the *Api types.
for f in $(src rpc/api); do
  sed -nE 's/^func \([a-z]+ \*?([A-Za-z]+(Api|API))\) ([A-Z][A-Za-z0-9]*)\(.*/rpc-method\t\1.\3/p' "$f"
done | grep -v '\.String$'

# Subscription topics.
sed -nE 's/^func \([a-z]+ \*?Api\) (To[A-Z][A-Za-z0-9]*|[A-Z][A-Za-z0-9]*)\(ctx context\.Context.*/subscription\t\1/p' rpc/api/subscribe/api.go

# Sporks.
sed -nE 's/^[[:space:]]*([A-Za-z0-9]+Spork)[[:space:]]*=.*/spork\t\1/p' common/types/spork.go

# Verifier rules: the per-rule methods of the account-block and momentum verifiers.
for f in verifier/account_block.go verifier/momentum.go; do
  sed -nE "s/^func \([a-z]+ \*?[A-Za-z]+\) ([a-z][A-Za-z0-9]*)\(.*/verifier-rule\t$(basename "$f" .go).\1/p" "$f"
done

# Wire protocol messages and protocol error codes.
sed -nE '/^[[:space:]]+Err/!s/^[[:space:]]+([A-Za-z]+Msg)([[:space:]]*=.*)?$/protocol-msg\t\1/p' protocol/protocol.go
sed -nE 's/^[[:space:]]+(Err[A-Za-z]+)([[:space:]]*=[[:space:]]*iota)?$/protocol-error\t\1/p' protocol/protocol.go

# Declared error values, by package: each is a refusal path that needs a case reaching it.
for f in $(src .); do
  case "$f" in ./build/*|./test/*) continue ;; esac
  sed -nE "s|^[[:space:]]*(Err[A-Za-z0-9]+)[[:space:]]*=[[:space:]]*(errors\.New\|fmt\.Errorf\|constants\.\|common\.\|New).*|error\t$(dirname "$f" | sed 's|^\./||').\1|p" "$f"
done

# Command-line flags and config-file fields.
sed -nE 's/^[[:space:]]+Name:[[:space:]]+"([a-z0-9.-]+)".*/cli-flag\t\1/p' app/flags.go
sed -nE 's/^[[:space:]]+([A-Z][A-Za-z0-9]+)[[:space:]]+[][*A-Za-z0-9.]+([[:space:]]+`.*)?([[:space:]]*\/\/.*)?$/config-field\t\1/p' node/config.go p2p/config.go
