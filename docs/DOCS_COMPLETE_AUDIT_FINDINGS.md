# Complete Docs Accuracy Audit Findings

Date: 2026-05-10
Branch audited: `docs/godoc-coverage`
Baseline for diff checks: `master...HEAD`
Output policy: findings-only report; no code, existing docs, generated docs,
scripts, configs, or source comments were edited.

## Status Update — 2026-05-10 (post-audit)

The following findings have been addressed in a follow-up commit on the same
branch. Verification re-run: `make doc-lint` passes (0 issues), `go test ./...`
passes, `gofmt -l .` empty, `git diff --check master` clean.

| Finding | Status | Resolution |
|---|---|---|
| P1 `README.md` Go version 1.16 → 1.20 | Resolved | `README.md:9` updated to "Go (version 1.20 or later)". |
| P1 / P5 `AGENTS.md` dead file paths | Resolved | Where-to-look table now points at `verifier/account_block.go`, `verifier/momentum.go`, and `rpc/api/{ledger,stats,utils}.go`. |
| P2 Generator includes unexported and generated internals | Resolved | `--include-unexported` removed from both `gomarkdoc` invocations in `scripts/gen-api-docs.sh`. `docs/api/` now mirrors the exported pkg.go.dev surface only. |
| P3 Generated docs too large for agent-readable review | Resolved | `docs/api/chain/genesis/README.md` 110,878 → 287 lines. Total tree 150,395 → ~22,856 lines (≈85% reduction). `embeddedGenesisStr`, protobuf `rawDesc`, and unexported JSON ABI literals no longer rendered. |
| P10 Trailing whitespace in generated markdown | Resolved | Generator now strips trailing whitespace from `docs/api/**/*.md` after generation (`sed -i.bak` step in `scripts/gen-api-docs.sh`). `git diff --check master` is clean. |

Findings still open (deferred): P1 pkg.go.dev wording, P1 send/receive
liveness phrasing, P2 placeholder `doc.go` files, P3 generator faithfully
reproduces false source comments, P4 placeholder package docs, P5 ledger
liveness wording, P6 wrong-symbol comments in `vm/embedded/definition/`, P7
generic embedded-implementation method comments, P8 generic RPC endpoint
comments, P9 protocol bridge inaccurate storage-loader comments, P9 P2P/RPC
server placeholder comments, P10 untracked `docs/DOCS_ACCURACY_REVIEW.md`.

## Executive Summary

The documentation branch materially improves godoc coverage and the basic
verification checks are mostly healthy: `make doc-lint`, `go test ./...`, and
`gofmt -l .` passed in this audit. Every Go package directory found by `find`
has a `doc.go`.

The docs are not yet accurate enough to hand to another AI as a source of
truth. The largest risks are generated docs that expose noisy internal data,
root navigation entries that point to nonexistent files, and widespread
placeholder comments that are rendered into `docs/api`. The generated docs are
also too large for the stated agent-readable goal: 46 README files, 150,395
total lines, with `docs/api/chain/genesis/README.md` alone at 110,878 lines.

Overall classification: **incomplete with confirmed inaccuracies**.

## Methodology

Non-mutating checks and inspections performed:

- `git status --short`
- `git diff --check master...HEAD`
- `gofmt -l .`
- `GOWORK=off GOCACHE=/private/tmp/go-zenon-gocache GOTMPDIR=/private/tmp make doc-lint`
- `GOWORK=off GOCACHE=/private/tmp/go-zenon-gocache GOTMPDIR=/private/tmp go test ./...`
- path-reference checks over root docs and docs policy files
- package/doc inventory with `find`, `rg`, `wc`, and targeted source reads
- generated-doc noise checks for protobuf raw descriptors and embedded genesis
- targeted code/doc spot checks in ledger, verifier, VM embedded definitions,
  VM embedded implementations, RPC API, and protocol bridge

Checks result summary:

| Check | Result | Notes |
|---|---:|---|
| `make doc-lint` | Pass | `0 issues.` |
| `go test ./...` | Pass | Completed successfully with temp cache env vars. |
| `gofmt -l .` | Pass | No output. |
| `git diff --check master...HEAD` | Fail | Trailing whitespace in generated embedded-definition README. |
| Go package directories with `doc.go` | Pass | 45 package dirs, 45 `doc.go` files. |

## Project Findings

### P1 Navigation and Root Docs

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| `AGENTS.md` points agents to missing files | P2 | 0.99 | inaccurate |

Evidence:

- `AGENTS.md:94` references `verifier/account_block_verifier.go`.
- `AGENTS.md:95` references `verifier/momentum_verifier.go`.
- `AGENTS.md:101` references `rpc/api/network.go` and `rpc/api/utility.go`.
- Actual files include `verifier/account_block.go`, `verifier/momentum.go`,
  `rpc/api/ledger.go`, `rpc/api/stats.go`, and `rpc/api/utils.go`.

Impact: an AI agent following the root map is sent to dead paths for validation
and RPC work.

Recommended follow-up: update the where-to-look table to point at existing
files and, for RPC, describe the actual namespace split.

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| `README.md` build prerequisite conflicts with `go.mod` | P2 | 0.98 | inaccurate |

Evidence:

- `README.md:9` says Go version 1.16 or later.
- `go.mod:3` declares `go 1.20`.
- `AGENTS.md:8` and `llms.txt:5` say Go 1.20+.
- `.github/workflows/doc-lint.yml:41` uses Go 1.20.

Impact: a reader may attempt an unsupported Go version.

Recommended follow-up: align `README.md` with Go 1.20+.

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| pkg.go.dev wording overstates generated-doc equivalence | P3 | 0.9 | ambiguous |

Evidence:

- `README.md:23` says `docs/api` publishes to pkg.go.dev after merge.
- `docs/api/README.md:6-7` says the same content publishes to pkg.go.dev.
- `scripts/gen-api-docs.sh:50` uses `--include-unexported`; pkg.go.dev does
  not present the same checked-in markdown or the same unexported/internal
  surface.

Impact: another agent may assume `docs/api` and pkg.go.dev are equivalent.

Recommended follow-up: clarify that source godoc comments publish to pkg.go.dev,
while `docs/api` is a checked-in review artifact with different rendering
rules.

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| Send/receive invariant is phrased as guaranteed liveness | P3 | 0.85 | ambiguous |

Evidence:

- `ARCHITECTURE.md:27` says every send eventually has exactly one receive.
- `verifier/account_block.go:407-438` enforces a real send and rejects an
  already-received send.
- `chain/account/mailbox/mailbox.go:91-127` shows sends can remain pending in
  the mailbox until consumed.

Impact: the code enforces "at most one receive" and tracks pending sends; it
does not locally guarantee eventual consumption.

Recommended follow-up: rephrase as "at most one receive is enforced; pending
sends remain in the recipient mailbox until consumed."

### P2 Documentation Tooling and Policy

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| Generator includes unexported and generated internals | P2 | 0.99 | generated-noise |

Evidence:

- `scripts/gen-api-docs.sh:48-53` runs gomarkdoc over `./...` with
  `--include-unexported`.
- `scripts/gen-api-docs.sh:78-84` repeats `--include-unexported` for libznn.
- `docs/api/chain/nom/README.md` includes
  `file_chain_nom_protobuf_proto_rawDesc`.
- `docs/api/common/types/README.md` includes
  `file_common_types_protobuf_proto_rawDesc`.
- `docs/api/consensus/storage/README.md` includes protobuf raw descriptor
  helpers.
- `docs/api/chain/genesis/README.md:113-117` includes `embeddedGenesisStr`.

Impact: agent-facing docs are dominated by implementation artifacts that do not
help explain the public API and can distract or mislead reviewers.

Recommended follow-up: filter generated files and large embedded literals from
the rendered artifact, or split internal/unexported docs into a separate
artifact.

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| Style/tooling claims exhaustive quality, but placeholders remain | P2 | 0.95 | incomplete |

Evidence:

- `docs/STYLE.md:3` claims exhaustive coverage for every package, type,
  function, method, and constant.
- `.golangci.yml:4-6` says the full rollout documented every package.
- Placeholder package docs remain in `cmd/libznn/doc.go`, `cmd/znnd/doc.go`,
  `common/crypto/doc.go`, `vm/embedded/definition/doc.go`,
  `vm/embedded/implementation/doc.go`, and `vm/embedded/tests/doc.go`.

Impact: lint passes, but the quality bar described by policy docs has not been
met.

Recommended follow-up: either complete the placeholder docs or change the
status language to say the rollout is still incomplete.

### P3 Generated API Docs

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| Generated docs are too large for agent-readable review | P2 | 0.99 | generated-noise |

Evidence:

- `docs/api` has 46 generated README files totaling 150,395 lines.
- `docs/api/chain/genesis/README.md` is 110,878 lines.
- The next largest generated files are `vm/embedded/definition` at 5,022 lines,
  `vm/embedded/implementation` at 3,883 lines, `rpc/server` at 2,857 lines,
  `p2p` at 2,289 lines, and `rpc/api/embedded` at 2,203 lines.

Impact: the docs are difficult for humans or AI agents to audit end to end.

Recommended follow-up: cap or exclude embedded literals and generated/internal
symbols before treating `docs/api` as an agent-facing artifact.

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| Generated docs faithfully reproduce false source comments | P2 | 0.99 | inaccurate |

Evidence:

- `vm/embedded/definition/bridge.go:1031-1044` has comments for unrelated
  helpers attached to `UnwrapTokenRequest` fields.
- The same comments appear in
  `docs/api/vm/embedded/definition/README.md:4558-4572`.
- `protocol/chain_bridge.go:74-128` has storage-loader comments on protocol
  sync methods.
- The same comments appear in `docs/api/protocol/README.md:588-615`.

Impact: regenerating docs does not fix accuracy; source comments must be
semantically corrected first.

Recommended follow-up: treat generated docs as a mirror, not a source of truth.
Fix source comments, then regenerate.

### P4 Package Overview Docs

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| Package doc coverage exists, but several package docs are still placeholders | P2 | 0.95 | incomplete |

Evidence:

- `find` found 45 Go package directories and 45 `doc.go` files.
- Six package docs still state that documentation is being filled in
  incrementally.

Impact: package-comment lint passes, but the docs are not complete enough for
the stated agent-use goal.

Recommended follow-up: replace incremental placeholders with the full template
sections required by `docs/STYLE.md`.

### P5 Ledger, Chain, Consensus, and Verifier

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| Ledger overview is broadly accurate, but liveness wording needs correction | P3 | 0.85 | ambiguous |

Evidence:

- `verifier/account_block.go:407-438` validates receive `FromBlockHash`,
  verifies the send exists, checks receiver mismatch after enforcement height,
  and rejects already-received sends.
- `chain/account/mailbox/mailbox.go:91-127` tracks unreceived and pending
  sends.

Impact: the model is useful, but "every send eventually" reads stronger than
what the verifier/store code can enforce.

Recommended follow-up: keep the model, but distinguish enforced safety from
network/client liveness.

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| Validation file names in agent map are stale | P2 | 0.99 | inaccurate |

Evidence:

- `verifier/account_block.go` and `verifier/momentum.go` contain the verifier
  implementations.
- `AGENTS.md:94-95` points to nonexistent verifier filenames.

Impact: another agent auditing validation may begin in the wrong place.

Recommended follow-up: update root navigation.

### P6 VM Embedded Definitions

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| Bridge definition comments annotate the wrong symbols | P2 | 0.99 | inaccurate |

Evidence:

- `vm/embedded/definition/bridge.go:1031` says
  `UnwrapTokenRequest is part of the package's public API`.
- `vm/embedded/definition/bridge.go:1033` places a
  `GetUnwrapTokenRequestByTxHashAndLog` comment on
  `RegistrationMomentumHeight`.
- `vm/embedded/definition/bridge.go:1043` places a
  `GetUnwrapTokenRequests` comment on `Signature`.

Impact: rendered docs show false descriptions for bridge request fields.

Recommended follow-up: line-review `vm/embedded/definition/bridge.go` and then
the rest of `vm/embedded/definition/`; replace generic or wrong-symbol comments
with field/type semantics.

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| Placeholder patterns are widespread in definition files | P2 | 0.95 | incomplete |

Evidence:

- Suspicious placeholder/comment-template search found 197 Go source hits.
- Highest definition clusters include `vm/embedded/definition/bridge.go` with
  35 hits, `common.go` with 21, `liquidity.go` with 14, `pillars.go` with 10,
  `accelerator.go` with 9, and `htlc.go` with 7.

Impact: many rendered docs are grammatically valid but not semantically useful.

Recommended follow-up: audit embedded definitions contract by contract.

### P7 VM Embedded Implementations

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| Generic method comments omit real contract behavior | P2 | 0.9 | incomplete |

Evidence:

- `vm/embedded/implementation/common.go:213-253` documents
  `DepositQsrMethod` generically even though the code requires QSR, requires a
  positive amount, and accumulates `QsrDeposit`.
- `vm/embedded/implementation/common.go:256-310` documents
  `WithdrawQsrMethod` generically even though it rejects non-zero amount,
  returns `ErrNothingToWithdraw` for empty deposits, deletes the deposit, and
  emits a QSR refund block.
- `vm/embedded/implementation/bridge.go:245-267` labels message-hashing helpers
  as storage loaders, though they construct signing payloads and hashes.

Impact: another agent reading docs may miss critical validation, state-change,
and descendant-block behavior.

Recommended follow-up: verify each embedded method's `GetPlasma`,
`ValidateSendBlock`, and `ReceiveBlock` comments against implementation code.

### P8 RPC API

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| RPC endpoint comments are too generic or misleading | P2 | 0.95 | incomplete |

Evidence:

- `rpc/api/embedded/bridge.go:132-154` says `GetAllNetworks` loads a record,
  but the implementation also validates page size, gets frontier context,
  loads the network list, slices a page, and returns count/list metadata.
- `rpc/api/embedded/bridge.go:156-160` uses a generic public-API placeholder for
  `NetworkInfoList`.
- `rpc/api/embedded/plasma.go:245-261` says request/response types and the PoW
  endpoint load records from storage, but the endpoint computes required plasma
  and difficulty.

Impact: RPC docs hide pagination, computed fields, validation, nil/error
behavior, and endpoint semantics.

Recommended follow-up: audit each embedded RPC namespace and document endpoint
contract behavior rather than storage access.

### P9 Protocol, P2P, and Sync

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| Protocol bridge comments invent storage records | P2 | 0.98 | inaccurate |

Evidence:

- `protocol/chain_bridge.go:74-77` returns all uncommitted account blocks but
  says it loads a `Transactions` storage record.
- `protocol/chain_bridge.go:86-97` fetches momentums and returns hashes but
  says it loads a `BlockHashesFromHash` record.
- `protocol/chain_bridge.go:99-117` constructs `DetailedMomentum` with
  prefetched account blocks but says it loads a `Block` record.
- `protocol/chain_bridge.go:128-131` fetches momentum by height but says it
  loads a `BlockByNumber` record.

Impact: protocol/sync docs are misleading exactly where another agent needs to
understand downloader/fetcher behavior.

Recommended follow-up: replace storage-loader comments with protocol role
descriptions and then spot-check `protocol/downloader` and `protocol/fetcher`.

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| P2P and RPC server contain low-information public-API comments | P3 | 0.85 | incomplete |

Evidence:

- Placeholder search found hits in `p2p/message.go`, `p2p/peer.go`,
  `p2p/rlpx.go`, `p2p/nat/*.go`, and `rpc/server/*.go`.
- Examples include `ReadMsg is part of the receiver's public API` and
  `ExternalIP is part of the receiver's public API`.

Impact: these comments satisfy lint but do little for agent-readable docs.

Recommended follow-up: lower priority than embedded/RPC/protocol bridge, but
replace placeholders where they feed public docs.

### P10 Hygiene and Audit Consistency

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| Existing audit says trailing whitespace was fixed, but it still fails | P3 | 0.99 | inaccurate |

Evidence:

- `docs/GODOC_AUDIT.md:48` says generated markdown trailing whitespace was
  removed.
- `git diff --check master...HEAD` still reports trailing whitespace in
  `docs/api/vm/embedded/definition/README.md` at lines 386, 536, 546, 745,
  1023, 1024, and 1246.

Impact: branch audit docs do not match current verification output.

Recommended follow-up: remove trailing whitespace via source/generation fix or
update the audit to reflect the failing check.

| Finding | Severity | Confidence | Classification |
|---|---:|---:|---|
| Pre-existing untracked audit artifact is outside the branch baseline | P3 | 0.8 | ambiguous |

Evidence:

- Initial `git status --short` showed `?? docs/DOCS_ACCURACY_REVIEW.md` before
  this report was created.

Impact: reviewers may confuse branch-tracked docs with local findings artifacts.

Recommended follow-up: decide whether to add, replace, or remove the older
untracked report before merge. This audit left it untouched.

## Subsystem Status Matrix

| Project | Status | Primary reason |
|---|---|---|
| P1 Navigation/root docs | Needs fixes | Dead paths, stale Go version, ambiguous liveness wording. |
| P2 Tooling/policy | Needs generator decision | Lint passes, but generator includes unexported/generated internals. |
| P3 Generated API docs | Not ready as source of truth | Huge output and mirrored false comments. |
| P4 Package docs | Incomplete | Coverage exists, but placeholder package docs remain. |
| P5 Ledger/chain/consensus/verifier | Mostly plausible with wording gaps | Safety checks align, liveness wording overstates enforcement. |
| P6 VM embedded definitions | High risk | Wrong-symbol and placeholder comments are concrete. |
| P7 VM embedded implementations | High risk | Generic method comments omit behavior. |
| P8 RPC API | High risk | Endpoint semantics hidden by generic storage-loader comments. |
| P9 Protocol/P2P/sync | Mixed | Protocol bridge has confirmed inaccurate comments. |
| P10 Hygiene/audit consistency | Needs fixes | Branch audit conflicts with `git diff --check`. |

## Handoff Checklist for Another AI

Use this order to address findings without mixing concerns:

1. Fix P1 and P10 first: root navigation, Go version, liveness wording, and
   whitespace/audit mismatch.
2. Decide P2/P3 generator policy before reviewing every generated line:
   exported-only vs internal docs split; generated-file exclusion; embedded
   genesis exclusion.
3. Fix source comments first, then regenerate `docs/api`; do not hand-edit
   generated markdown except as an emergency review artifact.
4. Audit P6 by embedded definition file, starting with `bridge.go`.
5. Audit P7 by embedded contract group: common methods, Pillar/Sentinel/Stake/
   Token/Plasma/Spork, Swap/Accelerator, HTLC, Bridge/Liquidity.
6. Audit P8 by RPC namespace, documenting pagination, computed fields, nil
   behavior, errors, and state context.
7. Audit P9 by protocol role: bridge, downloader, fetcher, p2p message/peer/NAT.
8. Re-run:
   - `git diff --check master...HEAD`
   - `gofmt -l .`
   - `make doc-lint`
   - `go test ./...`
9. Verify final status has no unrelated working-tree artifacts.

