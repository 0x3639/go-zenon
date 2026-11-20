# S-7 acknowledgement-recency driver cases — 2026-11-19 on `90635a5`

Commit under test: `90635a5` (branch `test/spork-cases` on top of
`testing/framework` `abe1fa6`). Environment: linux/amd64, docker compose
devnet, Go 1.24.1 image build. Added cases: `TestSporkEdge` (S-7, S-7b) in
`test/e2e/sporkedge_test.go`.

## What was run

| Variant | Command | Result |
|---|---|---|
| staged | `E2E_RPC=http://localhost:36997 go test -tags e2e ./test/e2e/ -run TestSporkEdge -count=1 -v` | S-7, S-7b PASS (`coverage-staged.tsv`) |
| stock | same | S-7, S-7b PASS (`coverage-stock.tsv`, log `e2e-sporkedge-stock.log`) |
| staged, full suite | `go test -tags e2e ./test/e2e/ -count=1 -v -timeout 40m` | 101 subcases PASS; the only FAIL was S-7b of the first, superseded version of the case (`e2e-full-staged.log.gz`) |

Deviation from the procedures: this host runs a mainnet node on
35997/35998, so the devnet published RPC on 36997 through a local port
overlay (the D1 workaround from the 2026-11-18 run); the driver ran against
`http://localhost:36997`. The staged devnet was started with the
`compose.variant.yml` overlay as in P10; stock with the plain compose file.

## Case results

- **S-7** refuses a user block that acknowledges the canonical momentum one
  below the highest enforcement height, at publish, with
  `chain.ErrStaleMomentumAcknowledged`; the refusal names the spork first
  enforced there with the lowest id. On staged (highest enforcement:
  EpochRewardTermsSpork at 80) the refusal named
  `…0007` = EpochRewardTermsSpork; on stock (highest: DynamicPlasmaSpork at
  30) it named `…0006` = DynamicPlasmaSpork.
- **S-7b**: the refused block consumed no account height; the same account
  received its funding with a fresh acknowledgement and the chain advanced
  (balance read back over RPC).

## Findings written into the case comments

- The refusal is only observable on the account's **first** block: an
  established account chain hits the older-than-previous rule first, with a
  message that also contains "momentum-acknowledged" (a substring match
  alone passes for the wrong rule).
- The acceptance edge "acknowledge the momentum at the enforcement height"
  is not reachable through RPC on a chain already past the enforcement: a
  receive must acknowledge a momentum at or above its from-block's
  confirmation height (`verifier/account_block.go`, fromHash rule), and any
  funding confirmed after the enforcement pins the account above it. That
  edge belongs to an activation-positioned run (S-2) and is pinned by the
  unit tests in `chain/acknowledgement_recency_test.go`.

## Coverage

`test/inventory/record.py` merged both runs: 5 tokens, newly covering
`error chain.ErrStaleMomentumAcknowledged`, `verifier-rule
account_block.acknowledgementRecency` and `spork DynamicPlasmaSpork` /
`spork EpochRewardTermsSpork`, all class E7. `check.sh` total moved from
191 to 195 of 501 items.

Static: `go build ./...`, `go vet -tags e2e ./test/e2e/` and `gofmt -l
test/e2e/` clean at `90635a5`.
