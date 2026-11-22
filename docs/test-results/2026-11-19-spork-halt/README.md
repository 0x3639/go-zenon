# S-4 / X-4 on staged: a node without EmbeddedRefundSpork at the enforcement height

Run 2026-11-19 on linux/amd64, branch `test/spork-cases` (base `abe1fa69`),
genesis variant `staged` (ptlc at 60, embedded-refund at 70,
epoch-reward-terms at 80), six containers: the five devnet nodes plus an
`oldnode` running a binary with the EmbeddedRefundSpork entry removed from
`types.ImplementedSporksMap` (image `go-zenon-devnet-noref:latest`, built
from the same commit otherwise; config override `"P2PBackend": "libp2p"`
because a stock-config late joiner cannot sync past the libp2p swap —
zenon-network/go-zenon issue #105; the build recipe and the compose
overlay are committed at `test/devnet/oldnode/` and
`test/devnet/compose.oldnode.yml`).

Driver: `E2E_OLDNODE=1 E2E_DEVNET_GENESIS=$PWD/test/devnet/genesis/staged.json go test -tags e2e -run TestSporkUnknownBinary`
(57 s, all four subtests PASS; full log `e2e-run-sporkhalt.log`).

What passed, and what it pins:

- **S-4** — the old node synced from genesis and refused the
  enforcement-height momentum: it sat at 69, still running, with the
  designed "Detected an unimplemented spork" message. On **dev** the
  momentum pool rolls the insert back and the node stays up one below the
  height. On **released v0.0.10** the same condition called `os.Exit(2)`
  in the pool (measured on the released v0.0.10 binary in an upgrade-day
  rehearsal on a devnet) — the "exits with status 2" wording of TEST-PLAN
  S-4 is the released behavior; dev changed it (error-return changes
  around zenon-network/go-zenon issue #196).
- **X-4** — the five nodes with the spork produced past 70 while the old
  node never advanced again (asserted at 69 in S-4). The upgrade split is
  real.
- **S-4c** — the old binary on a data directory that already passed the
  enforcement height refused to start (`chain.Init`), exiting **1** via
  cli.go (v0.0.10 exited 2 from `chain.Init`). Recorded for the record.
- **S-4b** — recovery per the runbook: the stock binary on the same data
  directory reached the live frontier (140); the case records the volume
  height before the swap and asserts the first post-boot frontier is at
  least it, so a silent resync from genesis cannot pass.

Coverage: `spork EmbeddedRefundSpork E7:S-4` (coverage.tsv).
