# Godoc conventions — docs/coverage-v2

Distilled from the v1 audit (`docs/COMMENT_AUDIT/FINDINGS.md` on the
`docs/godoc-coverage` branch). Each rule below addresses a class of
mistake that recurred in v1 and that v2 must avoid. The rules apply to
every layer PR (PR-1 through PR-10) so voice and accuracy stay
consistent.

This file is the only style reference v2 authors load before drafting.
**Do not consult v1 godoc for a file before you have written its v2
godoc cold** — the comparison artifact only catches divergence if v2 is
truly independent.

---

## 1. State contracts, not internal mechanics

**DO** describe what callers can rely on: what the function returns, what
it consumes, what invariants it preserves, what errors it surfaces.

**DON'T** narrate the body. The reader already has the body open. A
comment that paraphrases the next ten lines is noise. A comment that
says "returns the matching record, or `ErrNotFound` if no entry exists
for `addr` in the current epoch" is signal.

---

## 2. No "loads from storage" boilerplate

The v1 audit's largest single class of error was the phrase **"loads X
from storage"** applied to functions that *compose*, *aggregate*,
*compute*, or simply *pack bytes*. Examples that were not actually
loading anything:

- RPC `embedded/*.go` methods that call a definition helper, paginate,
  and remap into a response struct.
- `vm/embedded/implementation/*` message-builders and hash helpers
  (they construct payloads; they do not query storage).
- `vm/embedded/definition/*` key constructors that return `JoinBytes`
  byte prefixes.

**Rule**: only use "loads from storage" if the function reads a single
identified record from a backing store and returns it largely intact.
For everything else, name the actual operation: "composes", "aggregates",
"scans the prefix range", "packs the ABI payload", "derives the key".

---

## 3. Single record vs scan vs aggregate

Three distinct shapes that v1 conflated:

| Shape | Wording |
|---|---|
| One record by key | "Returns the X for `id`, or `ErrNotFound`." |
| Iterate a prefix | "Iterates every entry under prefix P and yields ..." |
| Aggregate over scan | "Sums Y over all entries under prefix P." |

If the function name is plural (`GetAllPillarVotes`,
`UnlockLiquidityStakeEntries`), the doc must reflect the iteration.
Singular wording on plural operations was a recurring v1 mistake.

---

## 4. Error names must match what the code returns

The v1 audit found comments referencing **`util.ErrX`** where the code
actually returns **`constants.ErrX`** (and vice versa). When you mention
an error symbol by name, copy the exact qualified name from the source.
Don't paraphrase.

---

## 5. Caller vs admin vs receiver

Several v1 comments described authorization in caller-centric language
when the code actually checked for an administrator address or zeroed
admin fields:

- **`EmergencyMethod`** does not "nominate the caller" — it operates on
  the admin slot.
- **`UnlockLiquidityStakeEntries`** is administrator-only, not
  caller-driven.
- **`HashByNetworkClass`** is a free function, not a receiver method.

When documenting authorization, read the actual permission check and
name the role the code enforces (admin, signer, owner, pillar, etc.).
Don't default to "caller".

---

## 6. Builders / hash helpers / key constructors are not "loaders"

The pure functional helpers in `vm/embedded/*` and `common/db/*` —
message builders, hash helpers, key constructors that return
`JoinBytes(...)` prefixes — operate on inputs in memory. They neither
read nor write storage. Document them as the deterministic byte
transformations they are.

---

## 7. Godoc location and target

Doc comments must sit **immediately above** the symbol they document
(no blank line between comment and `func`/`type`/`var`/`const`). The
v1 audit caught misplaced godoc in `definition/bridge.go` attached to
the wrong identifier. After writing the doc, re-read the next line to
confirm the symbol matches.

For package doc, prefer a dedicated `doc.go` over a comment on
`package foo` in a source file. `doc.go` files survive refactors that
delete or rename the source file they would otherwise hang off.

---

## 8. Numeric and behavioral details

Specific v1 mistakes that fall under "say what the code does, not what
it sounds like":

- `DataPath` default is `~/.znn`, not `~/.zenon` (and OS-specific paths
  via `DefaultDataDir()`).
- `MaxSearchIndex` is wallet config that `FindAddress` ignores
  (`keystore.go` hardcodes 128). Either document the dead field as
  dead, or remove it.
- `greaterDifficulty` includes **equality**, so "above threshold"
  wording is wrong — use "at or above".
- `MomentumEventListener` callbacks run **synchronously on the caller's
  goroutine**, not on a "broadcast goroutine". `chain/momentum_events.go`
  is authoritative.
- `MinPeers` is unused on the dial path; `MinConnectedPeers` drives
  dynamic dials.

When in doubt, read the underlying function. The audit collected ~40
similar mistakes; treat the list above as illustrative, not exhaustive.

---

## 9. Test code

Test comments age fast. Confirm:

- The numeric expectation in the comment matches the assertion.
- "Manually set X" wording reflects what the test actually does
  (several v1 tests said "manually set chain identifier" while only
  checksumming a single field).
- Test bullet lists don't duplicate paths (the `token_test.go`
  donate-path bullet duplicated in v1).

---

## 10. Cryptography wording

The audit found `common/crypto` claiming **BLAKE2b** when the
implementation uses **SHA3-256, SHA-256, and legacy Keccak256**. When
documenting crypto packages, copy the exact algorithm name from the
implementation file — not from memory of what "the standard Ethereum
hashing" tends to mean.

Use "Keccak" only when referring to the legacy pre-FIPS variant. Use
"SHA3-256" when referring to the standardised variant. They are not
interchangeable in this codebase.

---

## What this file is not

- Not a list of every audit finding. It is the class-level lessons.
- Not a substitute for reading the code. The whole reason for v2 is
  that v1's mistakes mostly came from drifting away from the source.
- Not a comparison input. The comparison script
  (`scripts/compare-godoc.sh`) runs against v1's actual godoc, not
  against this file.
