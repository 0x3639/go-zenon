# go-zenon godoc + docs-site integration — Design

**Date:** 2026-06-11
**Status:** Approved by user (this session); supersedes the topology and
sequencing assumptions in `docs/zenon-docs-execution-plan.md` where they
conflict.

## Goal

Ship developer documentation for go-zenon where the package reference is
generated from godoc comments, the JSON-RPC API has a machine-readable
OpenRPC spec (interactive playground later), and the docs are consumable
by AI agents. Two blockers drive the design: (1) the go-zenon source
lacks sufficient godoc comments to generate a useful reference, so
commenting the code comes first; (2) a Docusaurus site already exists at
docs.0x3639.com (repo `0x3639/docs`), so the site work extends that repo
rather than creating a new one.

## Decisions (made 2026-06-11)

| Decision | Choice |
|---|---|
| Site repo | Extend existing `0x3639/docs` (no new zenon-docs repo) |
| Godoc text | Cold re-author; do NOT salvage the deleted v1 pass (still recoverable at `refs/pull/2/head` = 64eac34 if ever needed) |
| Commenting flow | Per-layer PRs into a long-lived `docs/godoc` integration branch on the 0x3639/go-zenon fork |
| Fork master | Keeps tracking upstream zenon-network/master cleanly; docs work never merges to master without an explicit decision |
| Docs build pin | `0x3639/go-zenon` at an exact sha on `docs/godoc`, bumped as layers merge; repo URL + ref live in one pin file so flipping to upstream later is a one-line edit |
| First pass scope | Layer 1 (`rpc/api` + `rpc/api/embedded` + `rpc/api/subscribe`) commented and PR'd, then site machinery + OpenRPC seed (`ledger`, `embedded.pillar`) |
| Deploy | Keep the existing 0x3639/docs deploy pipeline untouched; add a validate workflow alongside it |

## Amendments to the original execution plan

1. **`embedded.governance` namespace is missing** from the plan's spec
   list. It exists only on the fork's governance branch, not in any
   release; list it as *pending* and add the fragment when governance
   ships in a pinned ref.
2. **Generated reference uses CommonMark mode, not MDX escaping.**
   Docusaurus v3 `format: 'detect'` parses `.md` files as CommonMark,
   eliminating the `{`/`<` escaping failure mode the plan budgets for.
   Hand-written guides remain `.mdx`.
3. **No floating versions anywhere.** gomarkdoc, Docusaurus, OpenRPC
   tooling, and GitHub Actions are all pinned to exact versions
   (the plan said `gomarkdoc@latest` in one place — global pinning
   constraint wins).
4. **Pin file carries the repo URL and the ref** (two keys), not just a
   ref.
5. **`examples/go.mod` (Phase 2, later pass) uses
   `replace github.com/zenon-network/go-zenon => ../.build/go-zenon`**
   so snippets compile against exactly the pinned checkout (the module
   path belongs to upstream, so a plain `require` cannot pin the fork).
6. **PR previews become PR build-checks.** The existing site's deploy
   stays as-is; `validate.yml` runs fetch → generate → spec-validate →
   build on PRs.
7. **Never invent RPC example responses.** Methods whose examples cannot
   be captured from a live node ship schema-only with a loud log line;
   the conformance script backfills later. Node URL comes from
   `ZENON_NODE_URL`.

## Workstream A — comment go-zenon (this repo)

**Integration branch:** `docs/godoc`, created from current fork master
(which equals upstream master). Layer branches named
`docs/godoc-<layer>` branch off `docs/godoc` and merge back via small
PRs. The user's review pacing applies: sequential, one layer PR at a
time.

**Layer order** (1 is this pass; order of the rest may be re-prioritized
between passes):

1. `rpc/api`, `rpc/api/embedded`, `rpc/api/subscribe` — feeds both the
   generated reference and OpenRPC authoring
2. `common`, `common/types`, `common/db`, `common/crypto`
3. `chain` and subpackages
4. `vm`, `vm/abi`, `vm/constants`, `vm/vm_context`
5. `vm/embedded/definition`
6. `vm/embedded/implementation`
7. `consensus` and subpackages
8. `protocol`, `p2p` and subpackages
9. `node`, `wallet`, `zenon`, `pillar`, `verifier`, `pow`, `metadata`,
   `app`, `cmd/znnd`

**Authoring rules:**

- Cold-authored: comments are written by reading the current code, never
  by consulting the deleted v1/v2 text.
- Comment-only diffs by construction: no identifier, signature, or
  behavior changes in a docs PR. Verified mechanically (see gates).
- godoc conventions: every exported symbol gets a doc comment starting
  with its name; package comments on every package (in a `doc.go` where
  no natural home exists); document units, invariants, locking
  expectations, and error semantics where the code reveals them — not
  boilerplate restating the signature.

**Per-layer verification gates (all must pass before PR):**

1. `GOWORK=off go build ./...` and `GOWORK=off go vet ./...` clean.
2. Comment-only check: parse every changed `.go` file with `go/parser`
   on both sides of the diff, strip comments, normalize whitespace —
   zero non-comment token differences (script committed on the branch
   as `scripts/check-comment-only.sh`).
3. Doc-coverage lint: a minimal `.golangci.yml` (committed on
   `docs/godoc`; master currently has none) enabling revive's exported
   /package-comments rules, run scoped to the layer's packages. The
   layer's packages must be clean; other packages are not gated yet.

**Commit/PR style:** the user's multi-paragraph format — summary,
authoring rules applied, what is covered, notable findings about the
code, verification footer. GPG signing: wait for pinentry; never
`--no-gpg-sign`.

## Workstream B — extend 0x3639/docs (existing site repo)

Local clone as a sibling working directory. Additions (existing
content and deploy remain untouched):

```
docs/ (repo 0x3639/docs)
├── .github/workflows/validate.yml   # NEW: PR check — fetch, gen, spec-validate, build
├── PINNED_GO_ZENON                  # NEW: repo=<url>  ref=<sha on docs/godoc>
├── scripts/
│   ├── fetch-go-zenon.sh            # NEW: shallow-clone pin into .build/go-zenon; idempotent
│   ├── gen-godoc.sh                 # NEW: pinned gomarkdoc → docs/reference/packages/*.md
│   └── assemble-openrpc.mjs         # NEW: YAML fragments → specs/openrpc.json + validate
├── specs/openrpc/                   # NEW: ledger.yaml, embedded.pillar.yaml, components/
├── specs/openrpc.json               # NEW: assembled artifact — committed
├── docs/reference/
│   ├── index.md                     # NEW: hand-written landing page (committed)
│   └── packages/                    # NEW: generated, gitignored
└── static/openrpc.json              # copied from specs/ at build
```

- Generated package pages: CommonMark `.md`, Docusaurus frontmatter
  (title/slug/sidebar_label), "View source" link to
  `github.com/0x3639/go-zenon` at the pinned sha, autogenerated sidebar
  preserving package hierarchy. Only packages already commented on
  `docs/godoc` are generated (start: the rpc/api layer); list expands as
  layers merge.
- OpenRPC seed: `ledger` + `embedded.pillar` methods read from the
  pinned source; shared components (Address, Hash, HashHeight,
  AccountBlock, Momentum, AccountInfo, TokenStandard, pagination
  wrappers) under `specs/openrpc/components/`; OpenRPC 1.2.x;
  `info.version` = pinned ref; validated with `@open-rpc/schema-utils-js`.
- Examples captured live via `ZENON_NODE_URL` when available; otherwise
  schema-only (amendment 7).
- `package.json` gains `gen` (fetch + gen-godoc + assemble-openrpc) and
  the build script copies `specs/openrpc.json` → `static/`.

## Acceptance for this pass

1. `docs/godoc` branch exists with Layer 1 commented; all three gates
   pass; PR open from `docs/godoc-rpc-api` into `docs/godoc`.
2. In 0x3639/docs: `pnpm run gen && pnpm build` green locally; CI
   validate workflow green; reference section renders the rpc/api
   packages; `/openrpc.json` validates and is served by the built site.
3. Existing site content and deploy unaffected (build output diff
   limited to new sections).

## Later passes (unchanged from the execution plan, re-sequenced)

Remaining layers (Workstream A) → playground 3b (timeboxed
`@metamask/docusaurus-openrpc`, custom-component fallback) → guides +
compile-tested snippets (Phase 2) → remaining namespaces + conformance
(3a/3c) → AI consumability (Phase 4: llms.txt, raw markdown, AGENTS.md)
→ versioning + search (Phase 5).

## Out of scope for this pass

Playground UI, guides/snippet injection, conformance CI, llms.txt, MCP
server design, any merge of docs commits into fork master or upstream.
