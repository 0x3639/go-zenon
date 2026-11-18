# Devnet runs, 2026-11-18, `dev` at `924e6b8`

First runs of the merged `dev` tree on the docker devnet: the read-only smoke test (procedures P1 partly, P2 to P7 of [TESTING.md](../../TESTING.md)) and, on a second fresh devnet the same day, the end-to-end driver (P9).

## Environment

| | |
|---|---|
| Commit | `924e6b87f77828e6c5e492b1a04bc5595ed3acbc` (`v0.0.10-alphanet-732-g924e6b8`) |
| Binary | linux/arm64, built with Go 1.24.1 in `golang:1.24.1-alpine` by `test/devnet/build-image.sh`; SHA-256 `0793b664a4533a0f7ba471ee92ee78f49b1f4376921e014392c724bca7fd00b4` |
| Host | macOS, Colima (4 CPU, 8 GiB), docker compose |
| Network | stock `docker-compose.yml`: pillar, pillar2, pillar3, rpc, observer; syncnode added at height 46. Explorer not started |
| Genesis | `docker/devnet/genesis.json` unchanged; `devnet-keygen --verify-genesis` passes |

## Results

| Case | Result | Detail |
|---|---|---|
| `go build ./...` (host, go1.27.1) | pass | |
| `go test ./... -count=1` (host) | pass | 45 packages |
| `go vet ./...` (host) | known | six unkeyed-literal warnings in `p2p/legacy` |
| one-chain | pass | same hash on all five nodes at height 66 |
| three-producers | pass | 24 / 22 / 21 momentums |
| cadence-10s | **fail** | one 20 s interval, between heights 29 and 30. Finding 1 |
| libp2p swap at height 20 | pass | pillar 1's log shows "libp2p swap complete" at enforcement (the other nodes' log files were not read for this line); all five nodes had 4 to 5 peers and one chain afterwards, and no interval longer than 10 s around height 20 |
| dynamic-plasma-version-2 | pass | enforcement 30, version 2 from 31 |
| spork-az, htlc, libp2p, dynamic-plasma | pass | |
| sync on five nodes | pass | state 2, 4 to 5 peers |
| 17 RPC getters | pass | list in `test/devnet/smoke.py`; full output in [devnet-smoke.json](devnet-smoke.json) |
| late joiner, stock config | **fail** | 0 peers and height 1 after 90 s. Finding 2 |
| late joiner, `P2PBackend: "libp2p"` | pass | frontier in 15 s, same hash as the RPC node |
| error or critical log lines | pass | 0 on all six nodes after 13 minutes |

31 of 32 smoke checks passed on the final run (height 68), with the late joiner on the override.

## End-to-end driver (P9)

Fresh devnet, same image, driver started at height 33. Per-case results: [e2e-run.log](e2e-run.log).

| Run | Result | Detail |
|---|---|---|
| Normal run, ten groups in parallel | 88 of 88 cases pass | 13 minutes. Ledger 23, Plasma 8, Token 10, Stake 5, HTLC 11, Pillar 9, Sentinel 7, Accelerator 6, Spork 4, spork ladder and swap 5 |
| Destructive run, `E2E_DESTRUCTIVE=1` | 3 of 3 pass | SP-5 to SP-7: a spork no binary implements is activated; both endpoints stop at height 89, one below its enforcement height, and are still there a minute later |
| An earlier attempt the same day | void | 48 cases passed, then the chain stopped at height 83. The driver's own spork case caused it (Finding 3). Three expectations in the cases were wrong and were corrected: the refusal text for an unknown token, the enforcement height of a new spork, and the assumption that Bridge and Liquidity are off (Finding 4) |

What the 88 cases showed on the network: sends and receives of ZNN, QSR and a new token with exact balances and the same momentum on both endpoints; payment by fused plasma and by proof of work; 30 refusals at publish checked by message (plasma, data length, signature, public key, hash, chain identifier, version, block type, position in the chain, acknowledged momentum, receive rules); contract calls refused at publish or failed at the contract as each case expects; an amount sent with a failed call refunded through one descendant block (HTLC, pillar, sentinel); HTLC unlock, proxy unlock, deny and allow, reclaim before and after expiry.

Error-level log lines during the normal run: 72 on five nodes (pillar 26, observer 24, pillar2 12, rpc 6, pillar3 4). All are `error while applying account-block` from the handler, on blocks of embedded contracts, with reason `previous block is missing` or `momentum-acknowledged points to missing momentum`. Every case still passed and both endpoints agreed, so the blocks were applied later. Not yet explained: whether this is a contract block arriving by gossip before its predecessor or its momentum, and whether error is the right level for it.

## Genesis variants (P10)

The same image and driver on three more chains, each started empty. Per-case results: [all-on](e2e-run-all-on.log), [mainnet-like](e2e-run-mainnet-like.log), [all-off](e2e-run-all-off.log).

| Variant | Sporks enforced at the start | Result | Groups skipped because their contract is not live |
|---|---|---|---|
| `all-on` | all eight | 100 of 100 cases pass | none |
| `mainnet-like` | Accelerator, HTLC, Bridge and Liquidity | 90 of 90 pass | PTLC |
| `all-off` | none | 73 of 73 pass | Accelerator, HTLC, PTLC |

What the variants added:

- PTLC, ten cases on `all-on`: create and unlock with an Ed25519 signature, a BIP-340 signature and a secp256k1 scalar; proxy unlock that cannot redirect the payment; a scalar lock that pays only its fixed destination; expiry; the reclaim lock, where the plain reclaim is closed and the signed reclaim needs the right signature, the right account and the expiry.
- A contract that is not live refuses every spork-gated method on every variant (PTLC on `stock` and `mainnet-like`; Bridge, HTLC, PTLC and the gated methods of Accelerator and Liquidity on `all-off`).
- The ledger, plasma, token, stake, pillar, sentinel and spork cases give the same results with and without dynamic plasma, libp2p, EmbeddedRefund and EpochRewardTerms, apart from the differences listed in Finding 5.

The first pass of `mainnet-like` and `all-off` had two and four failing cases. All six were expectations written for the stock devnet that do not hold before a spork (Finding 5); the cases now state the outcome per spork state, and the table above is the second pass on fresh chains.

Error-level log lines: 106 on `all-on`, 48 on `mainnet-like`, 23 on `all-off`, all of the two kinds described above for the stock run. Still not explained.

Not exercised by any variant: what EmbeddedRefund changes (the refund of a failed contract-to-contract send) and what EpochRewardTerms changes (reward terms per epoch). The driver has no case that produces a contract-to-contract send that fails, and an epoch is 24 hours. Both sporks were only shown not to disturb the other cases. The `staged` variant exists but has no cases yet for the heights before, at and after each spork.

## Test build with fast constants (P11)

A second image from the same commit with the files in `test/devnet/fast/` added through a build overlay: version `v0.0.10-alphanet-732-g924e6b8+fast-constants`, Go 1.24.1, linux/arm64. Devnet on the `fast` genesis (every spork from height 0, genesis time set at start), driver built with the same overlay. Per-case results: [e2e-run-fast.log](e2e-run-fast.log).

| Group | Result | What it showed |
|---|---|---|
| Plasma | 2 of 2 | a fusion cannot be cancelled inside its lock; after it the QSR comes back and the entry is gone; a second cancel fails |
| Stake | 3 of 3 | cancel after expiry returns the ZNN; the reward of the first epoch is credited and collected, and the uncollected amount is then zero; collecting with nothing to collect fails |
| Pillar | 5 of 5 | registration with the QSR deposit (the deposit is consumed); update by the owner; revoke refused inside the lock and accepted in the window, 15,000 ZNN returned; a delegator collects the reward of the first epoch; epoch history is recorded |
| Sentinel | 2 of 2 | deposit and withdraw; revoke in the window returns the ZNN and the QSR; a second revoke fails |
| Accelerator | 2 of 2 | a project is accepted by the pillars' votes; a phase is added, revised (which gives it a new id and resets its votes), voted and paid in ZNN and QSR |
| Bridge administrator | 7 of 7 | set network, token pair, metadata and orchestrator info; remove token pair and network; halt and unhalt; each refused for a non-administrator; input limits refused at publish |

21 of 21 on one fresh chain, in 20 minutes. The normal groups are skipped on this build by design.

Two earlier passes on the test build were needed to get the cases right, and one start failed outright:

- The first build used a 3-minute epoch and did not start: the consensus layer needs an epoch to be a whole number of 300-second election periods. A 5-minute epoch (one period) started and then panicked in every pillar at the first momentum, in `consensus/points.go`: the last completed epoch is computed as `(lastCompletedPeriod / periodsPerEpoch) - 1`, which on a fresh chain is -2 instead of -1 when the divisor is 1, and the node then asks for epoch -1. With the stock ratio of 288 this cannot happen. The build uses 10 minutes.
- First full pass, 17 of 21. The four failures were two wrong expectations: setting a bridge token pair is time-challenged (the first call opens the challenge, the same call applies it after the soft delay), and a second cancel of an already cancelled stake is accepted and pays zero (see below). The cases were corrected and the table is the pass on a fresh chain after that.

Seen on the test build and worth knowing:

- A second cancel of a cancelled stake returns success and makes the contract send a block of amount zero, which the account then has to receive. No ZNN is paid twice. The entry stays, with amount zero, until the next reward update removes it.
- `SetTokenPair` applied by its first call is possible only when an identical challenge was opened earlier and its delay has passed; the case skips with that message if it meets it on a reused chain.
- Rewards on a devnet depend on the genesis time. On the committed genesis, dated February 2025, the reward contracts are hundreds of epochs behind and nothing can be collected.

Error-level log lines, 156 on five nodes. 112 are the two `error while applying account-block` kinds seen on every run. 39 are `failed to update contracts ... not enough plasma on account` from the pillars' workers: only pillar 1's producer address has fused plasma in the devnet genesis, so pillars 2 and 3 cannot send the periodic contract updates and pillar 1 sends them all. That is a gap in the devnet genesis, not in the node. 5 are the bridge logging a refused halt from a non-administrator (the driver's own negative case).

Not covered on the test build: bridge wrap, unwrap and redeem; the TSS key change; guardians and the emergency path; liquidity administration and liquidity staking; sentinel and liquidity rewards; pillar reward collection by a pillar owner; the EpochRewardTerms rule itself (the update was made, its effect on the next epoch's split was not checked).

## Findings

### 1. A pillar skipped its slot when its producer event fired early

Pillar 3 was elected for the slot 10:04:10 to 10:04:20. Its manager handled the event at 10:04:09, refused it with `reason="current time is before start time"`, and was not triggered again, so the slot stayed empty. Evidence: [missed-slot-pillar3.log](missed-slot-pillar3.log).

The consensus loop waits `event.StartTime.Sub(time.Now())` and then broadcasts the event once (`consensus/consensus.go`); the pillar checks `common.Clock.Now().Before(e.StartTime)` and drops the event (`pillar/manager.go:98`). If the timer returns even slightly before the start time, the slot is lost. This code is the same on `master`; it is not something `dev` introduced. In this run the likely trigger is a clock adjustment inside the docker VM, which was not measured. It happened once in 80 slots.

Not yet done: reproduce on a host without a VM, and decide whether the pillar should wait out the remainder instead of dropping the event.

### 2. The stock late joiner cannot join after the libp2p swap

`make devnet-sync-up` after height 20 leaves the sync node at height 1 with no peers. It logs "libp2p spork not active; starting legacy (devp2p/RLPX) backend" while every other node has swapped to libp2p. Evidence: [late-joiner-p2p.log](late-joiner-p2p.log). This is open issue #105 reproduced on the committed devnet. None of the devnet configs sets `P2PBackend`.

With `"P2PBackend": "libp2p"` in the sync node's config it synced from genesis to the frontier in 15 seconds.

Consequence for a mainnet rollout: after the swap, a node started from an empty data directory with default config cannot sync. Either the default has to change or the operator instructions have to say so.

### 3. A spork that no binary implements stops every node at its enforcement height

The driver created a spork with the genesis spork key and activated it. Six momentums later every pillar logged `can't insert momentum because don't have all sporks implemented`, rolled its own momentum back and tried again each slot; the chain stayed one below the enforcement height. Evidence: [spork-halt-pillar1.log](spork-halt-pillar1.log). This is the designed stop (`chain/momentum_pool.go`), and it held on every node with no fork.

Two things to note. Whoever holds the spork key can stop the network with two transactions, and a stopped node needs a new binary; that is the trust the spork address already carries, stated here because the test shows it. And the log line and the returned error print the unimplemented sporks as memory addresses (`unimplemented=[0x40044eed70]`), not as ids or names; the names go only to standard output.

### 4. Bridge and Liquidity are live on the stock devnet although the genesis lists their spork as not activated

`embedded.spork.getAll` reports `bridge-liq` with `activated: false`. The contracts answer anyway, because HTLC is on and the spork ladder treats HTLC as implying Bridge and Liquidity (`vm/embedded/embedded.go`, `sporkVariantKey`). Cases LAD-1 and LAD-2 check it for every method. `bridge.SetRedeemDelay` is in the ABI but answers `method not found in the abi` in this state. The earlier statement in these documents that Bridge and Liquidity cannot be tested on the stock devnet was wrong and is corrected. What the stock devnet cannot do is run the bridge administrator's methods: the initial administrator is a fixed address whose key is not in the devnet.

### 5. Behaviour that differs by spork state, and one dead ABI entry

Seen by running the same cases on four chains. None of these is a fault in itself; each is something a client or a later test has to know.

- Call data with a trailing byte: with dynamic plasma it is refused with its own error. Without it the node drops the byte when it re-packs the data and then refuses the block with `account-block hash is different than the one computed`, which says nothing about the cause. Whether v0.0.10 answers the same way has not been compared.
- `plasma.SetVariables` exists only with dynamic plasma; before it the contract answers `method not found in the abi`.
- Accelerator and Liquidity exist from genesis with Donate (and Update for Liquidity). Their other methods answer `method not found in the abi` until their spork, while Bridge, HTLC and PTLC answer `contract doesn't exist`.
- `bridge.SetRedeemDelay` is declared in the bridge ABI and implemented nowhere, so it answers `method not found in the abi` in every spork state. The entry is on `master` as well. It is the one contract method in the inventory that can never have a passing call.

## Not covered by these runs

191 of 501 inventoried items now have at least one case (63 of 78 RPC methods, 83 of 91 contract methods, 45 of 205 error values); none has every edge class that applies to it, and for the Liquidity methods and the Bridge methods outside administration the only case is the refusal while the contract is not live. The eight contract methods with no case at all are `common.VoteByProdAddress`, `liquidity.Donate`, `liquidity.Update`, `pillars.RegisterLegacy`, `pillars.Update`, `sentinel.CollectReward`, `sentinel.Update`, `stake.Update`. No case yet for: bridge wrap, unwrap and redeem; liquidity staking and administration; sentinel and liquidity rewards; subscriptions; restart, rollback and concurrency classes; the spork edge on a running network; wire messages, verifier rules as such, flags and config fields. `-race`, shuffle, fuzzing and the `libznn` tag were not run. `test/inventory/check.sh` lists every item without a case.
