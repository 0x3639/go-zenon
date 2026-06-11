# Bugs found during the Layer-1 godoc pass (2026-06-11)

Found while reading `rpc/api/...` line-by-line to author godoc comments.
**None of these are fixed** — the godoc branch is comment-only by policy.
Each entry cites the code as of `docs/godoc-rpc-api` (master 667a69d).
Verify against current code before fixing; line numbers will drift.

## Likely real bugs

1. **`rpc/api/embedded/plasma.go` — `FusionEntryList.UnmarshalJSON` panics
   on fresh receivers.** The slice is sized from the receiver's *old*
   length (`make([]*FusionEntry, len(r.Fusions))`) but indexed over
   `aux.Fusions`. Decoding a non-empty list into a zero-value
   `FusionEntryList` → index out of range. Affects any client-side use
   of these wire types; the node itself only marshals.

2. **`rpc/api/embedded/accelerator.go` — `Project.UnmarshalJSON` has the
   same pattern**: `p.Phases = make([]*Phase, len(p.Phases))` then
   indexing by `aux.Phases` range. Panics decoding a project with phases
   into a fresh receiver.

3. **`rpc/api/embedded/plasma.go` — `GetRequiredPoWForAccountBlock`
   discards the error from `api.GetFrontierContext`** (shadowed/ignored),
   then dereferences the context. A failed frontier lookup → nil-pointer
   panic in the RPC handler.

4. **`rpc/api/ledger.go` — `PublishRawTransaction` ignores the error from
   `GetFrontierMomentum()`**; only `m == nil` is checked and `m` is
   otherwise unused.

5. **`rpc/api/ledger_types.go` — `addConfirmationInfo` overwrites an
   unchecked error**: `frontier, err := store.GetFrontierMomentum()` is
   followed by another assignment to `err` without checking the first.

6. **`rpc/api/stats.go` — `p2pPeerToPeer` mangles IPv6 addresses**:
   `strings.Split(remoteAddr, ":")[0]` yields `[` for `[::1]:1234`-style
   addresses. Use `net.SplitHostPort`.

7. **`rpc/api/embedded/sentinel.go` — `toSentinelInfo` swallows the
   frontier-context error and returns nil**, so `GetByOwner` can return
   `(nil, nil)` on an internal failure, indistinguishable from
   "no sentinel".

## Bridge / liquidity / htlc batch (Task A7 findings)

11. **`rpc/api/embedded/bridge.go:643-645` — unwrap list `sort.Slice`
    with no tie-break** on RegistrationMomentumHeight: requests
    registered at the same height paginate nondeterministically across
    calls. Related to the known unwrap-sort work in PR #57.

12. **`rpc/api/embedded/bridge.go:329-337` (also ~380-391, ~428-439,
    ~464-477) — token/finality errors inside page loops are swallowed
    with `continue`**, silently shrinking the returned page while Count
    stays full.

13. **`rpc/api/embedded/bridge.go:602-617` vs :329-340 — inconsistent
    error policy**: unwrap pagination hard-fails the whole call on a
    missing token pair while wrap pagination skips bad entries.

14. **`rpc/api/embedded/bridge.go:364,412` — wrap `ToAddress` filter is
    a case-sensitive compare against a lowercased stored value**;
    checksummed EVM addresses match nothing.

15. **`rpc/api/embedded/bridge.go` wrap/unwrap list endpoints don't
    check `RpcMaxPageSize`** (only `GetAllNetworks` does); pageSize is
    clamped only by list length.

16. **`rpc/api/embedded/bridge.go:256-264` — `getConfirmationsToFinality`
    declares an error return that is always nil**; and `:485` allocates
    a `List` slice that is immediately overwritten (dead allocation).

17. **`rpc/api/embedded/htlc.go:17-19` — `HtlcApi.z` and `HtlcApi.cs`
    fields are set but never used** (pattern repeats in other APIs).

## Subscribe package (Task A8 findings)

18. **`rpc/api/subscribe/api.go:66/89/178` — `Server.uninstallCh` has no
    senders anywhere**; the `case sub := <-s.uninstallCh` branch in
    `work()` is dead code (uninstall happens via the broadcast path).

19. **`rpc/api/subscribe/api.go:268` — `Api.subscribe` sends to
    `installCh` unconditionally**; after `Stop()` the worker no longer
    drains it, so once the 100-slot buffer fills, in-flight subscribe
    calls block forever (shutdown-window edge case).

## Observations / inconsistencies (lower priority)

8. **`rpc/api/embedded/accelerator.go` — `GetAll` does not cap
   `pageSize`** (`sort.SliceStable` + manual paging, no
   `RpcMaxPageSize` check), unlike every other paged handler in the
   package.

9. **`rpc/api/stats.go` — `NewStatsApi` tags its logger
   `module=net_api`** instead of something stats-specific; cosmetic
   logging inconsistency.

10. **`rpc/api/stats.go` — `OsInfo` silently ignores gopsutil errors**
    (fields stay zero, method never errors). Possibly intentional;
    callers cannot distinguish "zero" from "unavailable".
