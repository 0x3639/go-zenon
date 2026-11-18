# Testing go-zenon: framework and procedures

This document is the how. [TEST-PLAN.md](TEST-PLAN.md) is the what: the levels, the cases and the per-PR matrix. Anyone with the repository, Go and docker should be able to repeat every procedure here and get a result that can be compared with ours.

## 1. The rule that keeps gaps visible

No test effort can prove that everything was tested. What can be done is to make every untested item show up in a list that is generated from the code, so nothing is missed by forgetting it.

That is what `test/inventory/` does:

- `inventory.sh` reads the source and prints every testable item: 91 embedded-contract methods, 78 RPC methods, 4 subscription topics, 8 sporks, 35 verifier rules, 9 wire messages, 10 protocol error codes, 205 declared error values, 19 command-line flags and 42 config fields. 501 items at `924e6b8`.
- `coverage.tsv` records, for each item, the ids of the cases that exercise it on a running node.
- `check.sh` compares the two. It fails when an item has no case, when the code has an item the map does not list, or when the map lists an item the code no longer has.

```sh
test/inventory/check.sh          # report gaps; exit 1 while any remain
test/inventory/check.sh --sync   # after a code change: add new items, drop removed ones
```

State on 2026-11-18: 191 of 501 items have at least one case, from the smoke test and the end-to-end driver on four genesis variants and the test build. The other 310 are the work list, and so is every item whose cases do not yet span the edge classes below. Unit tests are not counted here on purpose: the map tracks what has been shown on a live network, and `go test -cover` tracks unit coverage separately.

A change that adds a contract method, an RPC method, an error value, a flag or a config field makes `check.sh` report it as `MISSING` until someone runs `--sync` and records a case. That is the mechanism that stops the plan from going stale.

### What counts as covered

An item is covered when cases exist for every edge class that applies to it. One passing call is not coverage.

| Class | Applies to | What to try |
|---|---|---|
| E1 Happy path | everything | the normal call, result read back through RPC on all nodes |
| E2 Boundaries | every numeric, length and list input | zero, one, the limit, the limit plus one, the type maximum, negative where the encoding allows it |
| E3 Wrong caller | contract methods, admin RPC | non-owner, non-administrator, a contract address, an address with no chain |
| E4 Wrong state | contract methods | before the precondition, after it has expired, twice in a row, after revoke or cancel |
| E5 Wrong asset or amount | payable methods | wrong token, zero amount, amount above balance, amount that overflows a total |
| E6 Malformed encoding | every decoder: ABI, JSON, protobuf, wire frames, key files, config | truncated, trailing bytes, wrong types, null where an object is expected, deep nesting, huge strings |
| E7 Spork edge | everything a spork gates | one momentum before enforcement, at enforcement, one after; a block that acknowledges a momentum from before it |
| E8 Restart | anything with stored state | stop every node in the middle of the flow, start again, finish the flow |
| E9 Rollback | anything a momentum can undo | fork the network so the block is rolled back, then re-applied or dropped |
| E10 Concurrency | pools, caches, subscriptions, servers | the same action from many clients at once; conflicting actions in one momentum |
| E11 Resource limits | servers, pools, queues | at the cap, one over the cap, a client that never reads, a client that disconnects mid-request |
| E12 Time | anything with a height or time window | one tick before the window opens, the last tick inside, the first tick after; clock skew between nodes |

Record cases in `coverage.tsv` as ids with the class in front, for example `E1:C-1 E4:C-3 E7:S-1`. An item with only `E1` is listed as partly covered in the results file.

## 2. What you need

| Tool | Version used here | Notes |
|---|---|---|
| Go | 1.23 or later on the host, with cgo | `go.mod` says 1.23; release binaries are built with 1.24.1. The driver (P9) runs on the host |
| docker with compose | any current | about 2 GB of free disk in the docker VM for the stock build |
| python3 | 3.9 or later | standard library only |
| git | any | the commit under test must be checked out |

With Colima the repository must be under your home directory, because only that is shared into the VM.

## 3. Procedures

Every procedure starts by writing down the commit: `git rev-parse HEAD`. A result is valid for that commit only.

### P1. Static checks and unit tests (levels L0 and L1)

```sh
go build ./...
go vet ./...                                  # six known warnings in p2p/legacy at 924e6b8
go test ./... -count=1                        # about 5 minutes, 45 packages
go test -race ./... -count=1                  # on an idle machine
go test ./... -count=5 -shuffle=on            # finds order-dependent tests
go build -tags libznn ./... && go test -tags libznn ./cmd/libznn/...
go run ./cmd/devnet-keygen --verify-genesis docker/devnet/genesis.json
```

Fuzzing, one target at a time:

```sh
go test ./vm/embedded/implementation -run '^$' -fuzz '^FuzzName$' -fuzztime 10m
```

List the targets with `grep -rn '^func Fuzz' --include='*_test.go' .`.

Pass: every command exits 0, except `go vet`, which passes when it reports nothing new.

### P2. Build the devnet image

Normal path:

```sh
make devnet-up        # builds with the repository Dockerfile and starts everything
```

When the docker VM is short on disk, or to pin the Go toolchain:

```sh
GOIMG=golang:1.24.1-alpine test/devnet/build-image.sh
```

The script compiles `znnd` in a Go container with the source mounted read-only (cgo is required), wraps it in a small alpine image tagged `go-zenon-devnet:latest`, and prints the binary's SHA-256 and `znnd version`. Check that the commit hash it prints is the one you wrote down.

### P3. Start the devnet

```sh
docker compose up -d --no-build pillar pillar2 pillar3 rpc observer   # after P2's script
# or: make devnet-up                                                  # stock path, includes the explorer
```

Five nodes on `172.30.0.0/24`, three pillars, chain id 69. RPC on `http://localhost:35997` and `ws://localhost:35998`; pillar 1 also on `35991`. Topology, addresses and the dev mnemonic are in [docker/devnet/README.md](../docker/devnet/README.md).

Genesis sporks: Accelerator and HTLC from height 0, libp2p at 20, dynamic plasma at 30. The `bridge-liq` spork is listed as not activated, but the Bridge and Liquidity contracts are live all the same: HTLC is on, and the spork ladder treats HTLC as implying them. PTLC, EmbeddedRefund and EpochRewardTerms are not in this genesis, so nothing they gate can be tested on the stock devnet (see TEST-PLAN.md section 8, genesis variants).

### P4. Smoke test

```sh
python3 test/devnet/smoke.py --height 45 --out smoke.json
```

It waits for the given height, then checks: one chain on all five nodes, three producers, 10-second cadence, momentum version 2 from the height after dynamic-plasma enforcement, the four genesis sporks active, every node synced with at least two peers, and one getter in each RPC namespace. Exit code 0 means all passed.

Expected time: about 8 minutes from a cold start, since height 45 is 450 seconds.

### P5. Late joiner

```sh
make devnet-sync-up                     # or: docker compose --profile sync up -d --no-build syncnode
make devnet-sync-status                 # stats.syncInfo on port 35999
python3 test/devnet/smoke.py --sync     # adds the two late-joiner checks
```

Pass: state 2, at least one peer, same momentum hash as the RPC node.

**Known failure at `924e6b8`:** started after height 20, the late joiner stays at height 1 with no peers. It starts on the legacy transport because its own chain has not reached the libp2p spork, while every other node has already swapped. This is issue #105. To continue past it, force the transport and restart:

```sh
docker exec znnd-devnet-syncnode sed -i 's/"Net": {/"Net": {\n        "P2PBackend": "libp2p",/' /root/.znn/config.json
docker restart znnd-devnet-syncnode
```

With that it reached the frontier in 15 seconds in our run. Record both results: the stock failure and the result with the override.

### P6. Collect evidence

Each node writes `/root/.znn/log/zenon.log`. Copy the logs before tearing down:

```sh
for n in pillar pillar2 pillar3 rpc observer syncnode; do
  docker cp znnd-devnet-$n:/root/.znn/log/zenon.log logs-$n.log 2>/dev/null
done
grep -cE 'lvl=(eror|crit)' logs-*.log    # any hit needs an explanation in the results file
```

### P7. Tear down

```sh
make devnet-down        # keep chain state
make devnet-down-wipe   # remove it; the next start reproduces the same genesis
```

### P8. Record the result

One directory per run: `docs/test-results/<date>-<short commit>/`. It holds a `README.md` with the commit, the environment, each case id with pass or fail, every deviation from the procedure, and the evidence files. Then update `test/inventory/coverage.tsv` for the items the run exercised and run `test/inventory/check.sh`.

A failed case is recorded as failed, with the log lines that show it and an issue number once one exists. It is not re-run until it passes and then recorded as a pass.

### P9. End-to-end driver

The driver in `test/e2e/` signs and publishes account blocks and reads the result back over RPC. It is Go, built on the repository's own `wallet`, `chain/nom`, `pow`, `vm/abi` and `rpc/server` packages, and sits behind the `e2e` build tag so `go test ./...` never starts it.

Start the devnet (P3), then:

```sh
E2E_COVERAGE=run.tsv go test -tags e2e ./test/e2e/ -count=1 -v -timeout 40m | tee e2e.log
test/inventory/record.py run.tsv     # merge the passed cases into coverage.tsv
test/inventory/check.sh              # the new gap count
```

| Variable | Default | Meaning |
|---|---|---|
| `E2E_RPC` | `http://localhost:35997,http://localhost:35991` | endpoints, comma separated. Blocks are published to the first; agreement checks read all of them |
| `E2E_CHAIN_ID` | `69` | chain identifier put into every block |
| `E2E_MIN_HEIGHT` | `33` | the driver waits for this height before it starts, so that the stock devnet is past dynamic-plasma enforcement (30) |
| `E2E_COVERAGE` | unset | file to write the coverage rows of passed cases to |

It takes about ten minutes. The groups run in parallel, each on its own fresh accounts, and every run derives new accounts and names from the start height, so the driver can be run again on the same chain. It uses the committed devnet mnemonic and must not be pointed at a network with real funds.

How to read a result:

- A case id is the part of the subtest name before the first underscore: `TestPlasma/PL-3_cancel_before_expiry_fails` is case `PL-3`. Ledger cases use the `B-` ids of TEST-PLAN.md section 5.2.
- A contract method can refuse in two places, and a case states which one it expects. **Refused at publish** means `ledger.publishRawTransaction` returned an error; the case checks the message. **Failed at the contract** means the block was accepted and the contract's receive block carries status 2; the reason is then only in the node log, so the case checks the status and that the state did not change.
- A failed call that carried an amount must refund it. The cases that cover this check for one descendant block on the contract's receive and for the sender's balance after receiving it.
- A case records the inventory items it exercised and the edge class, and only when it passes. That is where the rows in the coverage file come from; nobody types them.

One case is destructive and runs alone:

```sh
E2E_DESTRUCTIVE=1 go test -tags e2e ./test/e2e/ -run TestHaltOnUnknownSpork -count=1 -v
```

It activates a spork that no binary implements and checks that every endpoint stops one below the enforcement height. The chain cannot continue afterwards: run it last, collect the logs (P6), then wipe (P7). Never give a normal case the right to activate a spork.

To add a case: add a `t.Run("<id>_<what>", ...)` in the group it belongs to, use `callOK`, `callFail` or `callReject` for the outcome, read the state back with `get`, and name what it covers with `cover(t, "<class>", "<kind>", "<item>")`. A negative block that a wallet would never build is made with a `Mutator`, which changes the block before hashing, after the plasma fields are set, or after signing.

### P10. Genesis variants

The spork state of a chain is fixed by its genesis, so the same driver is run on several of them. A variant is the committed `docker/devnet/genesis.json` with a different spork list and nothing else changed: same accounts, pillars, balances and keys.

| Variant | Sporks enforced | What it stands for |
|---|---|---|
| `stock` | Accelerator and HTLC from 0, libp2p at 20, dynamic plasma at 30 | the committed devnet |
| `all-off` | none | the chain before its first spork |
| `mainnet-like` | Accelerator, HTLC, Bridge and Liquidity from 0 | what mainnet enforces today: legacy transport, no dynamic plasma |
| `all-on` | all eight; libp2p at 20 and dynamic plasma at 30, the rest from 0 | everything this binary implements |
| `staged` | as `all-on`, with PTLC at 60, EmbeddedRefund at 70, EpochRewardTerms at 80 | the new sporks arriving on a running network |

```sh
test/devnet/genesis.py --list                 # the variants and their heights
test/devnet/up.sh all-on                      # wipe, then start the five nodes on that genesis
E2E_COVERAGE=run.tsv go test -tags e2e ./test/e2e/ -count=1 -v -timeout 40m
test/devnet/down.sh logs-all-on               # copy the logs, count error lines, wipe
```

`up.sh` needs the image from P2 and starts from an empty chain every time. It selects the genesis through the compose overlay `test/devnet/compose.variant.yml`, which mounts the chosen file over the one in the image; no committed docker file changes. `genesis.py` writes the files to `test/devnet/genesis/`; check a new one with `go run ./cmd/devnet-keygen --verify-genesis <file>`.

The driver reads the spork list when it starts. It waits until three momentums after the last scheduled spork, runs only the groups whose contract is live, and for a contract that is not live it checks that every spork-gated method is refused. A case whose outcome depends on a spork says so in the case (`live["dynamic-plasma"]`). Record one results section per variant; a case that passes on one variant says nothing about another.

A variant cannot change what is compiled in. On every variant the bridge administrator is a fixed address whose key nobody on the devnet holds, an epoch is 24 hours, and stake, fusion, pillar and sentinel locks last hours to months. Cases that need those (bridge administration, rewards, expiry of long locks) run on the test build of P11.

### P11. Test build with fast constants

Some rules can only be reached after a day or more on a stock binary: an epoch is 24 hours, a stake lasts at least 30 days, a pillar is locked for 83 days. And the bridge administrator is a fixed address whose key nobody on the devnet holds. Those values are compiled in, so no genesis variant can change them.

The test build changes only those values. It is the same source with two extra files added at build time through a Go build overlay (`go build -overlay`); nothing in the node's source tree is edited. The files are `test/devnet/fast/constants.go.in` and `consensus.go.in`, and they are the complete list of differences:

| Value | Stock | Test build |
|---|---|---|
| Epoch | 8,640 momentums (24 hours) | 60 momentums (10 minutes) |
| Wait after an epoch before it is rewarded | 1 hour | 10 seconds |
| Minimum distance between contract updates | 300 momentums | 3 momentums |
| Fusion lock | 3,600 momentums | 12 momentums |
| Stake unit | 30 days | 1 minute |
| Pillar lock and revoke window | 83 days, 7 days | 2 minutes, 1 minute |
| Sentinel lock and revoke window | 27 days, 3 days | 2 minutes, 1 minute |
| Accelerator phase unit and voting period | 1 day, 14 days | 1 minute, 10 minutes |
| Bridge and Liquidity administrator | a fixed mainnet address | devnet account 9 |
| Bridge unhalt delay, administrator delay, soft delay | 6 hours, 2 epochs, 1 epoch | 6, 12 and 6 momentums |

The epoch cannot be shorter than 10 minutes. It has to be a whole number of election periods of 300 seconds, and with exactly one period per epoch a fresh node panics at its first momentum (the reason is written in `constants.go.in`).

```sh
FAST=1 test/devnet/build-image.sh             # image go-zenon-devnet-fast:latest; version ends in +fast-constants
test/devnet/up.sh fast                        # every spork from height 0, genesis time = now
test/devnet/fast/overlay.sh "$PWD" > build/fast-overlay.json
E2E_COVERAGE=run.tsv go test -overlay build/fast-overlay.json -tags e2e ./test/e2e/ -count=1 -v -timeout 60m
test/devnet/down.sh logs-fast
```

The driver has to be built with the same overlay, because it takes its expected values from the same constants. It compares its own constants with the node's version string when it starts and refuses to run if only one of the two is the test build. On the test build only the `TestFast` groups run; the normal groups are skipped, because their "before the window" cases assume the stock durations. Allow 30 minutes: the reward cases wait for the first epoch to end.

The `fast` genesis sets its genesis time to the moment it is generated. Epochs are counted from the genesis time, and the committed genesis is dated February 2025; on it the reward contracts would first have to work through every epoch since then, twenty per update.

What a pass on the test build means: the rule holds when its window is reached. It does not show that the stock durations are what they should be, and the test binary is not the binary under test for anything else. A binary built with these files must never join a network with other binaries on it.

## 4. Procedures still to be written

These need tooling that does not exist yet (TEST-PLAN.md section 8). Each gets a P-number here when its tool lands.

| Procedure | Needs |
|---|---|
| Bridge wrap, unwrap and redeem; TSS key change; guardians and the emergency path; Liquidity administration and liquidity staking; sentinel and liquidity rewards | driver cases on the test build (the build exists, P11). Wrap and unwrap need the driver to sign as the orchestrator with a test secp256k1 key |
| Before, at and after each new spork | driver cases written for the `staged` variant (the variant exists, P10) |
| Sync and network fault cases | fault peer; chaos script |
| RPC limits and subscriptions | load client for HTTP and WS |
| Mixed-version network | v0.0.10 image with devnet constants |
| Mainnet sync and state comparison | a host; RPC differ |

The smoke test is read-only. Everything that sends a transaction goes through the driver (P9).

## 5. Repeating our first run

The run of 2026-11-18 on `924e6b8` is in [test-results/2026-11-18-924e6b8/](test-results/2026-11-18-924e6b8/README.md). To repeat it: P2 with the script, P3 without the explorer, P4 at height 45, P5 started after height 45, P6, P7 with wipe. Then on a fresh devnet: P3, P9 normal run, P9 destructive run, P6, P7 with wipe. Then P10 for `all-on`, `mainnet-like` and `all-off`, and P11.
