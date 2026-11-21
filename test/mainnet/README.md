# Mainnet comparison tools for the go-zenon test framework

Two tools for [test level L6](docs/TESTING.md) (mainnet history): testing a
`dev` binary against a v0.0.10 reference node on mainnet. They are
standard-library Python 3, read-only, and live in their own package
(`test/mainnet/`) so they never mix with the devnet driver (which signs and
publishes transactions — do not run `test/e2e` or `test/devnet` against a
mainnet host). Both tools:

- refuse to run unless every endpoint reports chain identifier 1 and the
  mainnet momentum-1 hash `9e204601…8eeb0` (spec: 0x3639/go-zenon#17),
- emit one JSON line per comparison/finding (JSONL), a final summary by case
  id, and the go-zenon commit the tool runs from,
- retry transport-level failures (a node under sync can drop connections).

## `compare.py` — M-2: mainnet RPC state differ

```sh
python3 test/mainnet/compare.py \
    --ref http://127.0.0.1:35997 \          # v0.0.10 reference node
    --test http://127.0.0.1:36997 \         # dev binary under test
    --addr-count 1000 --addr-window 25000 \ # M-2c address derivation
    --out results.jsonl
```

- **M-2a**: full `ledger.getDetailedMomentumsByHeight` answers (momentum
  hash, changes hash, every account block) at each spork activation height
  and the one before it, 4,188,198-4,188,207, 14,567,886-14,567,888, and the
  frontier. Heights the test node has not synced to are `INFO`.
- **M-2b**: frontier momentum hash; same height with different hash is a
  `FAIL` (split chain).
- **M-2c**: frontier state. Account infos are derived deterministically
  (first-seen unique addresses from account blocks of the last
  `--addr-window` momentums on the test node), read in batches of
  `--batch-size`; a batch counts only if the frontier hashes of both nodes
  are identical before and after the batch. Then every `embedded.*` read
  method in `test/inventory/coverage.tsv` (61 methods) with lazily resolved
  arguments (pillar name, accelerator project/phase, bridge network/wrap/
  unwrap ids, ZNN zts, ...) and paging to the end (pageSize 1000).
- **M-2d** controls: self-self at height 100 (must match) and a planted
  height difference 100 vs 101 (must be detected).

Comparison semantics: node-local contextual fields (`numConfirmations`,
`token.totalSupply`/`maxSupply`, recursively incl. `pairedAccountBlock`) are
normalized before comparison. RPC-level errors are comparable answers:
identical errors on both sides are a PASS, an error on exactly one side is
INFO (`error-on-one-side` or `method-missing-on-one-side`), a difference in
the error surface is a FAIL. A batch/getter taken while the frontier moved
is INFO, never FAIL. A pruned reference node serves no account blocks for
old heights: recorded as `block-availability-mismatch`, not a difference.

Left out of the getter list (spec: listed): `stats.*` (node-local),
`ledger.getUnconfirmedBlocksByAddress` (unconfirmed pool), all write methods
and subscriptions.

**Validation** (results/ extract in docs/test-results/ on the PR): self-self
run against one v0.0.10 node — M-2a 15 PASS, M-2b PASS, account-info batches
PASS, 59/60 getters PASS with one `frontier-moved` INFO, controls 2 PASS,
exit 0; cross-run against a `dev@54f6bad` node mid-sync — all comparable
cases either PASS or INFO, exit 0.

## `scan.py` — M-3: mainnet history scanner

```sh
# hash-link walk over momentum headers 1..H (pages of 1000, verifies each
# page is height-contiguous and that page boundaries chain by previousHash)
python3 test/mainnet/scan.py --node http://127.0.0.1:35997 \
    --headers --to 14661812 --out scan.jsonl

# full account-block scan over a stated range (10 momentums per call):
# counts momentums and account blocks read, reports every
# ChangeTssECDSAPubKey send (selector 15a0c641 = first 4 bytes of the
# SHA3-256 of "ChangeTssECDSAPubKey(string,string,string)", same computation
# as vm/abi Method.Id) and every momentum/block with Difficulty >= 2^63
python3 test/mainnet/scan.py --node http://127.0.0.1:36997 \
    --blocks --from 1 --to 14661812 --out scan.jsonl
```

**Validation**: header walk 1..200,000 on a dev node — 200,000 momentums,
0 link breaks; block scan of 2,710 frontier momentums on a v0.0.10 node —
200 account blocks read, counts and frontier edge handled, exit 0.

**Limits**: a block-level scan of the full chain is ~1.5M RPC calls; run it
against the full-history node (the `dev` binary), in slices. A pruned node
serves account blocks only near its frontier.
