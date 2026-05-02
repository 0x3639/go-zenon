# Godoc Style Guide

The conventions every documentation PR follows. Coverage is **exhaustive**: every package, type, function, method, and constant — exported and unexported — has a godoc comment.

## Goals

1. Render cleanly on pkg.go.dev.
2. Be coherent enough that an LLM with no source access can answer architectural questions from docs alone.
3. Stay terse. A comment that doesn't add information is noise.

## Rules

### Package comments (`doc.go`)

Every package has a `doc.go` whose only contents are the package comment and `package xxx`. See [PACKAGE_TEMPLATE.md](PACKAGE_TEMPLATE.md). Required sections in order:

1. **Summary** — one sentence starting `// Package X ...`. This is what pkg.go.dev shows in indexes.
2. **Overview** — what the package does and why it exists. 2–6 sentences.
3. **Key Concepts** — definitions of vocabulary introduced here. Cross-link terms to `ARCHITECTURE.md` glossary.
4. **Usage** — typical entry points and call sequences.
5. **Related Packages** — explicit `[pkg.Symbol]` doc-links to upstream/downstream packages.

### Symbol comments

- Start with the symbol name: `// Foo does …`.
- State **what** and **why**, not **how**. The reader can read the body for *how*.
- Mark error returns: `// Returns ErrX if …`.
- Mark concurrency: `// Concurrency: safe for concurrent use.` or `// Concurrency: caller must hold chain.InsertLock.`.
- Mark invariants: `// Invariant: Height == PreviousHash.Height + 1.`.
- Reference other symbols with doc-links: `// See also [chain.Chain.AcquireInsert].`.

### Vocabulary

Use terms exactly as defined in `AGENTS.md` glossary. Never introduce synonyms (no "consensus block" — always "momentum"; no "transaction" for an account block when "account block" is more precise).

### Length

- Symbol comments: 1–3 sentences. If you need a paragraph, the doc belongs on a higher-level type or package comment.
- Package comments: scannable. If the package needs more than ~80 lines of overview, split into sub-packages.

### What not to write

- No filler ("This function returns the value of X", "Sets the X" — say *why* it's set).
- No restatement of the signature.
- No git/PR references ("added in #42", "fixes the bug from issue #93").
- No emojis.
- No code-history asides.
- No multi-paragraph docstrings where one sentence suffices.

### Examples

Worked examples for non-trivial APIs go in `example_*_test.go` files. Use Go testable examples (`func ExampleFoo()`) so they:
- Render on pkg.go.dev as collapsible "Example" blocks.
- Are compiled and run under `go test`.

### Generated files

`*.pb.go` files are not manually documented. They must carry a header comment matching `Code generated .* DO NOT EDIT.` so the `revive` `package-comments` rule recognizes them as generated and skips checks.

## Checklist for a Per-Package PR

Before opening:

- [ ] `doc.go` follows the template, all five sections present.
- [ ] Every exported and unexported type/function/method/constant has a comment.
- [ ] Every comment starts with the symbol name.
- [ ] No synonyms introduced; vocabulary matches the glossary.
- [ ] Doc-links (`[pkg.Symbol]`) used for every cross-reference.
- [ ] Concurrency / invariant prefixes used where applicable.
- [ ] `gofmt -l <pkg>` is empty.
- [ ] `go vet ./<pkg>/...` is clean.
- [ ] `go test ./...` passes.
- [ ] `golangci-lint run ./<pkg>/...` has no doc-related findings.
- [ ] `go doc -all ./<pkg>` reads well end-to-end.
- [ ] Spot-checked locally with `make docs` and a browser.

## Tooling Reference

- `make docs` — run godoc server on `:6060` for local preview.
- `make doc-lint` — run `golangci-lint` with the doc-only ruleset.
- `go doc -all ./pkg/...` — render the rendered doc for a package.
