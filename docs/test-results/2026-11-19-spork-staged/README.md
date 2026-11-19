# S-1/S-2/S-5 staged spork enforcement — 2026-11-19

Case: `test/e2e/sporkstaged_test.go` (`TestSporkStagedEnforcement`), branch
`test/spork-cases` on top of `testing/framework` (`abe1fa6`). Environment:
linux/amd64, the five-node docker devnet on the `staged` genesis
(libp2p@20, dynamic-plasma@30, ptlc@60, embedded-refund@70,
epoch-reward-terms@80), driver and node both built from `abe1fa6` stock
constants, all five node RPCs published on host ports
(E2E_ALL_RPC, local overlay; the stock compose collides with a mainnet
node on 35997, see results 2026-11-18 finding D1).

Covers TEST-PLAN.md S-1, S-2 and S-5 (edge class E7) for the five sporks
that arrive while the network runs. The run is positioned before the
first staged height (`E2E_MIN_HEIGHT=12`), so every boundary is observed
live, one momentum before, at and one after the enforcement height.

## Results

| Subtest | Result | Time |
|---|---|---|
| S-1_before_enforcement_gated_rules_off | PASS | 0.02 s |
| S-2_libp2p_at_enforcement | PASS | 79.5 s |
| S-2_dynamic_plasma_at_enforcement | PASS | 100.6 s |
| S-2_ptlc_and_embedded_refund_at_enforcement | PASS | 399.4 s |
| S-2_epoch_reward_terms_at_enforcement | PASS | 100.6 s |
| S-5_restart_after_activation | PASS | 14.6 s (standalone re-run) |

Logs: `e2e-run-sporkstaged.log` (main run, 719.6 s: all subtests; S-5 first
attempt failed on a wrong expectation in the case itself — it asserted
momentum 30 to be Version 2, see the finding below — and was fixed),
`e2e-run-sporkstaged-s5.log` (the fixed S-5 re-run alone, 14.6 s, PASS).
Coverage: `coverage-sporkstaged.tsv` (main run), `coverage-sporkstaged-s5.tsv`
(re-run); 16 tokens merged into `test/inventory/coverage.tsv`
(spork items 2/8 → 5/8; total 191 → 198 of 501).

## What each subtest asserts

- S-1: below every staged height the spork is scheduled but not in force;
  for Dynamic Plasma the consensus-visible gate is pinned: momentums carry
  Version 1 with zero `NextFusionPrice`/`NextWorkPrice` (which the momentum
  verifier requires pre-spork, verifier/momentum.go). No container has ever
  restarted.
- S-2: at each enforcement height all five nodes return the same momentum
  hash at H-1, H and H+1 (no fork), all containers are running, none has
  ever restarted (no node exited).
- S-5: `docker restart` of all five nodes preserves the spork table exactly
  (ids, activation, enforcement heights), the pre-restart frontier momentum
  is still canonical (no resync) and the chain continues.

## Finding pinned by this run

**The Dynamic Plasma momentum-format switch lands one momentum past the
enforcement height.** A producer builds momentum N on its parent N-1's
spork state (pillar/worker_momentum.go chooses the version from
`IsSporkActive` at the previous momentum), so momentum 30 on the staged
genesis is still Version 1 with zero resource prices and the first
Version 2 momentum — with both prices at or above `dp.MinResourcePrice` —
is 31. The network accepts both, so the verifier applies the same
parent-based rule. A case that expects Version 2 *at* the enforcement
height fails on this tree.

## Not covered here

The gated send behaviour of EmbeddedRefundSpork (the stalled refund queue
of issue #271) needs a failing contract-to-contract send; its only live
trigger is a bridge wrap of a non-burnable Owned token and the bridge
administrator delays are epoch-scale on this genesis. That behaviour is
covered by `vm/embedded/tests/embedded_refund_test.go` and an external
lab rehearsal (zenon-resilience-lab, phase 27) instead. This case covers
what the staged network itself can show: boundary mechanics (S-1/S-2) and
restart survival (S-5).

## Reproduce

```
test/devnet/up.sh staged          # plus a local port overlay if 35997 is taken
E2E_RPC=http://localhost:35997 E2E_CHAIN_ID=69 E2E_MIN_HEIGHT=12 \
E2E_ALL_RPC=<five node RPC urls> \
go test -tags e2e ./test/e2e/ -run TestSporkStagedEnforcement -v
test/devnet/down.sh
```
