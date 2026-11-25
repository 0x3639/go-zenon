# Test framework for go-zenon

Everything here runs a real `znnd` network in docker and checks it from the outside. Nothing in this directory is built into `znnd`, and `go test ./...` does not start any of it.

Start with [docs/TESTING.md](../docs/TESTING.md): it has the procedures, step by step. [docs/TEST-PLAN.md](../docs/TEST-PLAN.md) says what is to be tested and why, and [docs/test-results/](../docs/test-results/) holds the results of each run.

| Directory | What it is | Procedure |
|---|---|---|
| `devnet/` | builds the devnet image, starts the five-node devnet on a chosen genesis variant, and a read-only smoke test | P2 to P7, P10 |
| `devnet/fast/` | two files added at build time to make a test binary with a short epoch and short locks | P11 |
| `e2e/` | the end-to-end driver: signs and sends transactions and reads the results back over RPC | P9 |
| `inventory/` | lists every testable item from the source and tracks which of them have a case | section 1 |
| `mainnet/` | read-only RPC differ and history scanner for comparing a `dev` binary against a v0.0.10 reference node on mainnet (L6) | L6 in TEST-PLAN.md |

The shortest way to see it work, from the repository root, with Go, docker and python3 installed:

```sh
test/devnet/build-image.sh                 # about 5 minutes the first time
test/devnet/up.sh all-on                   # five nodes, every spork
E2E_COVERAGE=run.tsv go test -tags e2e ./test/e2e/ -count=1 -v -timeout 40m
test/devnet/down.sh logs                   # keep the logs, remove the chain
test/inventory/check.sh                    # what still has no case
```

The driver uses the mnemonic committed for the devnet in `cmd/devnet-keygen`. Do not point it at a network with real funds.

To add or change a case, see "To add a case" under P9 in TESTING.md. A case that passes records what it covered by itself; `inventory/record.py` merges that into `inventory/coverage.tsv`.
