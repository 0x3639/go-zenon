# Godoc Branch Audit

This document records the review of `docs/godoc-coverage`, whose scope was to
add godoc coverage without changing runtime behavior.

## Findings

1. Non-comment Go tokens had drifted from `origin/master`.
   - `vm/embedded/definition/swap.go` removed a trailing space inside the
     `jsonSwap` raw ABI string.
   - `chain/store/account.go` added an interface parameter name to
     `AddChainPlasma`.
   - `chain/store/momentum.go` added an interface parameter name to
     `IsSporkActive`.

2. Doc lint enforcement was incomplete.
   - `.github/workflows/doc-lint.yml` still had `continue-on-error: true`.
   - The workflow installed `golangci-lint` v1.55.2, while `.golangci.yml`
     uses v2 configuration syntax.
   - `make doc-lint` did not set `GOWORK=off`, so it failed in checkouts that
     sit under an unrelated `go.work`.

3. Some embedded-contract docs were inaccurate or placeholder-like.
   - `GetPlasma` comments said methods loaded Plasma records from storage,
     while the implementations return values from the supplied
     `constants.PlasmaTable` or method-specific configuration.
   - Several embedded-method comments only said they were part of the
     receiver API and did not describe validation, execution, or fee behavior.
   - The embedded dispatcher docs described origin Liquidity as donation-only,
     but origin also wires Liquidity `Update`; accelerator tier adjusts
     CollectReward plasma costs and opens Liquidity `Fund`/`BurnZnn`.

4. Generated markdown had trailing whitespace in
   `docs/api/vm/embedded/definition/README.md`.

5. Branch-local artifacts were present that were not part of godoc coverage.
   - `.claude/scheduled_tasks.lock`
   - `.golangci.bck.yml`

## Remediation Applied

- Restored the non-comment Go token drift so the remaining Go source changes
  are comments/package docs only.
- Updated embedded dispatcher and embedded-contract method comments to match
  the implementations.
- Updated checked-in `docs/api` markdown for the godoc text touched by this
  remediation.
- Removed generated markdown trailing whitespace reported by `git diff --check`.
- Made doc lint enforceable by removing `continue-on-error`, installing
  `golangci-lint` v2.6.2 in CI, and running lint with `GOWORK=off`.
- Removed the stale `.golangci.bck.yml` backup file. The `.claude` lockfile is
  also removed from the working tree.

## Verification

Run these checks before merging:

```sh
git diff --check origin/master...HEAD
make doc-lint
GOWORK=off golangci-lint run --config=.golangci.yml --build-tags libznn ./cmd/libznn ./app ./metadata
GOWORK=off go test ./...
```

The no-runtime-code-change check should compare non-comment Go tokens between
`origin/master` and `HEAD`; only comment/doc additions should remain.
