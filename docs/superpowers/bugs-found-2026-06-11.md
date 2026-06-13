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

# Layer-2 additions (2026-06-12, common packages)

20. **`common/ticker.go` — `ToTick` on a time before startTime** converts
    a negative int64 to uint64 (astronomical tick number instead of an
    error); a sub-second interval is a divide-by-zero panic
    (`uint64(interval.Seconds())` == 0).

21. **`common/task.go` — `t.finish()` is not deferred**: if a task's
    action panics, `Finished()`/`Wait()` never resolve (moot today only
    because the unrecovered panic kills the process).

22. **`common/types/address.go` (`DeProtoAddress`) — panic message cites
    `HashSize` (32) while checking `AddressSize` (20)**; misleading
    diagnostics for malformed AddressProto payloads. Also cosmetic:
    `SetBytes` error string has a double space.

23. **`common/db/versioned_db.go` — `ldbManager.Add` silently returns
    nil** when the transaction doesn't build on the frontier, while
    `memdbManager.Add` returns an error; asymmetric failure modes.

24. **`common/db` — empty values conflate with deletion tombstones**:
    `enableDeleteDB.Put(key, []byte{})` stores exactly the tombstone
    byte; through a historical view a key rolled back to absent returns
    `([]byte{}, nil)` and `Has` → true instead of ErrNotFound, while
    iterators correctly skip it. Latent inconsistency if any caller ever
    stores empty values.

25. **`common/db/versioned_db.go` — truncated error message**
    `"can't find previous for "` (identifier never formatted in).

# Layer-3 additions (2026-06-12, chain packages)

26. **`chain/momentum/ledger_store.go:66` — swallowed `Apply` error**:
    `if err := ...Apply(patch); err != nil { return nil }` returns nil
    instead of err while applying an account-block patch during
    momentum insertion. A failed state write would be silently
    ignored. Same pattern at :96 (`setBlockConfirmationHeight`).
    **Highest-severity finding of the docs effort so far.**

27. **`chain/momentum/embedded.go` — `ComputePillarDelegations`
    discards errors** from `getAllDelegations()` and
    `GetActivePillars()` (`, _ :=`), silently producing empty
    delegation sets on storage errors.

28. **`chain/genesis/account_block.go:192` — `entry.Save` error
    ignored** in genesisSporkContractConfig; every other Save in the
    file is wrapped in `common.DealWithErr`.

29. **`chain/genesis/shared_tests.go` — `checkAccountBalance` passes
    vacuously** when the address has no GenesisBlocks entry at all,
    even when non-zero amounts are required.

30. **`chain/account_pool.go` (canRollback) — "previous mismatch"
    branch reuses the "missing previous" error string**; misleading
    diagnostics.

31. **`chain/account/mailbox` — `GetUnreceivedAccountBlockHashes`
    with atMost == 0 underflows** to effectively unlimited (only
    caller passes 500). Also `mailbox.go:20` unreachable return after
    panic (pre-existing vet finding).

32. **Dead code observations**: `momentumStore.SetFrontier` /
    `accountStore.SetFrontier` and the mailbox's permanent
    `unreceivedBlockPrefix` index have no readers anywhere
    (write-only); `nom.AccountBlockMarshal` carries a vestigial
    unexported `producer` field; `nom.AccountBlockHeaderComparer`
    is a non-strict (<=) comparer, safe only because headers are
    unique.

33. **`chain/genesis/config.go` (`ReadGenesisConfigFromFile`) — panic
    during state construction yields (nil, nil), and the caller
    treats it as success**: the deferred recover only logs; with
    err == nil, node/config.go:132-134 prints "Loaded a valid genesis
    config" and proceeds with a nil genesis. A genesis file that
    panics NewGenesis mid-build is reported as valid. Suggested fix
    (from Layer-3 PR review): make the recover path return
    ErrInvalidGenesisConfig.

# Layer-4 additions (2026-06-12, vm-core packages)

34. **`vm/supervisor.go` (`GenerateAutoReceive`) — verifier called on a
    nil block**: the internal error from generateEmbeddedReceive is
    checked only AFTER `s.verifier.AccountBlock(block)`; on an internal
    error block is nil and the verifier dereferences it — nil-pointer
    panic, and unlike the Apply* paths this method has no recover.

35. **`vm/plasma.go` (`AvailablePlasma`) — unit-mismatched cap**:
    compares computed plasma against MaxFussedAmountForAccountBig (a
    QSR amount, 5e11) and would return that amount as plasma.
    Unreachable today (max real plasma ~10.5e6) but wrong if hit.
    Also note the constant's "Fussed" spelling.

36. **`vm/supervisor.go` (`packBlock`) — dead double-sign**: two
    identical consecutive `if signFunc != nil` blocks; the first sign
    pass is discarded work (harmless since ChangesHash is not part of
    the account-block hash, so both signatures are identical).

37. **`vm/abi/error.go:101,104` — error messages garbled**:
    errArrayOffsetOverflow swaps its offset/len format arguments;
    errInsufficientLength formats a []byte with %d instead of its
    length. Also "varible" typo at :40.

38. **`vm/vm_context/lifecycle.go` (`Done`) — does not nil the
    accountStoreSnapshot** (unlike Reset); latent with today's
    single-window usage.

39. **User-block `ChangesHash` is never validated** (observation from
    Layer-4 PR review follow-up): verifier/account_block.go has no
    ChangesHash check, and vm only compares it on the regenerated
    ContractReceive path. An externally published user block can carry
    an arbitrary ChangesHash that is stored and served over RPC as-is.
    State integrity is still protected by the momentum-level
    ChangesHash (which is validated and covered by the momentum hash);
    the per-block field is informational/garbage-prone for user blocks.

# Layer-5 additions (2026-06-12, vm/embedded/definition)

40. **`vm/embedded/definition/accelerator.go` — project/phase storage
    prefixes are bytes 12/13, not the intended-looking 1/2**: the
    `_ byte = iota` sits in a const block with eleven preceding specs,
    so iota starts at 11. Harmless (writers and readers share the
    constants) but surprising for raw storage inspection; verified
    empirically. Worth a dedicated const block if ever touched.

41. **`vm/embedded/definition/swap.go` (`GetSwapAssets`) — an
    empty-valued key aborts the entire listing** with
    ErrDataNonExistent instead of being skipped (same pattern exists
    in pillars' GetLegacyPillarList). Latent: swap entries are never
    stored empty today.

42. **Observations**: `GetAllPillarVotes` scans the whole pillarVote
    prefix and filters by id in memory although the key layout
    supports a per-id prefix scan (inefficiency);
    `unmarshalRewardDepositHistoryEntryKey` reads epoch bytes before
    checking the address error; `Phase.ToProjectMarshal` marshals a
    phase despite the name; `definition.SetTokenPairParam` is
    unreferenced by the implementation.

# Layer-6 additions (2026-06-12, vm/embedded/implementation)

43. **`vm/embedded/implementation/liquidity.go`
    (`updateLiquidityRewards`) — epoch emission silently skipped under
    backlog**: the MaxEpochsPerUpdate cap counts BLOCKS (2 per epoch,
    so effective cap 10 epochs) and is checked AFTER
    checkAndPerformUpdateEpoch has already advanced and persisted the
    epoch marker — the capping iteration's mint blocks are never
    emitted, losing that epoch's emission. Reachable only via the
    legacy pre-spork update with 10+ epochs of backlog.

44. **Duplicate guardian addresses widen the emergency-recovery
    threshold**: neither bridge nor liquidity NominateGuardians
    rejects duplicates; a duplicated address still casts one vote
    (the proposal loop breaks at the first matching slot) but the
    majority threshold counts slots (len/2), so duplicates make
    administrator recovery harder and can deadlock it (e.g. 6 slots /
    3 unique guardians → 4 votes needed from 3 possible voters).

45. **`vm/embedded/implementation/accelerator.go` — AddPhase and
    UpdatePhase validate neither Amount nor TokenStandard**; tokens
    sent along with these calls are silently kept by the contract
    (every other non-donation method enforces zero-amount).

46. **`vm/embedded/implementation/accelerator.go` — Save/Delete
    errors discarded throughout** (~12 call sites) where other
    contracts wrap with common.DealWithErr; a silent DB failure
    would diverge state.

47. **`common.Big0` aliasing**: stake.go and swap.go store the shared
    common.Big0 pointer into entries that are then packed
    (stakeInfo.Amount, deposit.Znn/Qsr, RewardDeposit.Znn); latent
    hazard if any future code mutates those big.Ints in place.

48. **`vm/embedded/implementation/pillars.go` — reward computation
    debt**: computeDetailedPillarReward ignores the GetPillarsList
    error; a dead never-written `distributed` map is sorted and
    iterated for debug output; computePillarRewardForEpoch reads
    detail.Pillars[name].Weight before its !ok guard (latent panic);
    checkPillarPercentages compares uint8 < 0 (always false).

49. **`vm/embedded/implementation/liquidity.go`
    (`SetAdditionalReward`) — missing CheckSecurityInitialized**
    unlike SetTokenTuple/ChangeAdministrator/Emergency; with security
    uninitialized its time challenge runs with the default MinSoftDelay.

50. **`vm/embedded/implementation/accelerator.go` — voting-window
    edge**: a project whose votes reach the threshold only after the
    14-day window, with no update run inside the window, transitions
    to Closed without its votes ever being tallied; the "passed
    voting period" log fires while the window is still open.

# Layer-7 additions (2026-06-12, consensus)

51. **`consensus/chain_ticker.go` — mismatch error prints the wrong
    block's hash**: the comparison is against the LAST block's hash
    but the message reports blocks[0].Hash as "got".

52. **`consensus/api.go` (`GetPillarDelegationsByEpoch`) — panics via
    common.DealWithErr on TickMultiplier failure** where sibling
    paths in the same method return errors; unreachable today (both
    tickers share the genesis anchor) but inconsistent.

53. **Observations**: `consensus/api/pillar_stats.go` bakes the
    "ExceptedBlockNum" misspelling into the JSON wire field
    (exceptedBlockNum) — kept for compatibility, worth an alias if
    the API is ever versioned; `Point.LeftAppend`'s error message
    renders ranges as [x,y) while the semantics are (prev, end].

# Layer-9 additions (2026-06-13, node shell)

54. **`wallet/keystore.go` (`DeriveForFullPath`) — returned path is
    always empty**: the named return `path` is never assigned, so it
    returns "" alongside the (correct) key and error. `DeriveForIndexPath`
    inherits this.

55. **`wallet/keystore.go` (`FindAddress`) — ignores the configurable
    MaxSearchIndex**: it bounds the search by the package const
    `maxSearchIndex` (128), not the `Manager`'s `Config.MaxSearchIndex`,
    so the config knob has no effect on address search.

56. **`wallet/manager.go` (`Lock` / `IsUnlocked`) — IsUnlocked stays
    true after Lock**: Lock sets `decrypted[path] = nil` instead of
    deleting the entry, and IsUnlocked tests key presence, so it keeps
    reporting unlocked after a Lock (and GetKeyStore would then return
    a nil store with a nil error). Docs updated to describe the actual
    behavior; bug left for a fix branch.

57. **Cosmetic error-string typos** (verifier/pillar): pillar/errors.go
    "finish time time"; verifier/account_block.go "from-block-hash is
    nor provided"; verifier/errors.go ErrMTimestampNotIncreasing "is is
    lower".

# Layer-9 PR-review additions (2026-06-13)

58. **`wallet/keystore.go` (`KeyStore.Zero`) — does not actually wipe
    secrets**: it sets Entropy/Seed to nil and Mnemonic to "", which
    only drops references. The underlying entropy and seed byte arrays
    are never overwritten and the mnemonic string cannot be scrubbed by
    reassignment, so the secret material persists in memory until GC.
    A method named Zero implies in-place wiping. Security weakness:
    overwrite the byte slices before niling, and accept the string
    limitation (or keep the mnemonic only as bytes).

59. **`zenon/mock/zenon.go` — Stop does not restore global state it
    overwrote** (test-pollution hazard): common.Clock is replaced with
    a mockClock and never restored, so after a mock node Stops the
    global clock still points at the stopped chain; and
    initialEpochDuration is captured AFTER newMockZenon sets
    consensus.EpochDuration to the custom value, so Stop's restore is a
    no-op (the prior global value is lost). A later test in the same
    process that relies on the real clock or the default epoch duration
    sees the leftover state. Capture both globals before overriding,
    and restore common.Clock in Stop.
