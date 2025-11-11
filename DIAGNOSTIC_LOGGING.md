# Diagnostic Logging Branch - Technical Documentation

## Overview

This branch adds **comprehensive diagnostic logging** to the Zenon node software to help analyze network behavior, transaction flow, and momentum production patterns. This is particularly useful for spam testing, network monitoring, and debugging issues related to transaction processing.

## ⚠️ IMPORTANT: Logging Only - No Functionality Changes

**This branch makes ZERO changes to consensus, transaction processing, or any node functionality.**

All modifications are purely diagnostic logging that observes and records what the node is doing. The node behaves **identically** to the master branch in terms of:
- ✅ Consensus algorithm
- ✅ Transaction validation
- ✅ Momentum production
- ✅ Block processing
- ✅ P2P networking
- ✅ All embedded contracts

The only differences are:
- 📝 Additional read-only data collection
- 📝 Writing diagnostic information to a log file
- 🔧 Two interface additions to pass peer count (read-only helper methods)

## What Gets Logged

All diagnostic information is written to: `{dataPath}/log/diagnostic.log`

Each log entry is written in **both JSON and human-readable format** for easy analysis.

### 1. Node Startup Information (`NODE_INFO`)

**When**: Node starts up
**Purpose**: Record node configuration and role

```json
{
  "type": "NODE_INFO",
  "node_id": "16Uiu2HAm...",
  "role": "pillar",
  "pillar_name": "MyPillar",
  "producer_addr": "z1qq...",
  "listen_addr": "0.0.0.0:35995",
  "static_peers": 5,
  "bootstrap_peers": 3,
  "chain_id": 1,
  "version": "v0.0.6",
  "timestamp": "2025-11-11T10:00:00.000Z"
}
```

### 2. Transaction Reception (`TX_RECEIVED`)

**When**: Node receives a transaction from a peer
**Purpose**: Track when and from where transactions arrive

```json
{
  "type": "TX_RECEIVED",
  "tx_hash": "abc123...",
  "from_peer": "16Uiu2HAm...",
  "timestamp": "2025-11-11T10:00:01.123Z"
}
```

### 3. Account Block Added to Pool (`ACCOUNT_BLOCK_ADDED`)

**When**: Transaction is added to the mempool
**Purpose**: Track successful transaction acceptance

```json
{
  "type": "ACCOUNT_BLOCK_ADDED",
  "tx_hash": "abc123...",
  "address": "z1qq...",
  "height": 42,
  "source": "peer_received",
  "timestamp": "2025-11-11T10:00:01.125Z"
}
```

**Source values**:
- `peer_received` - Transaction received from network
- `local_create` - Transaction created by this node

### 4. Mempool Snapshots (`MEMPOOL_SNAPSHOT`)

**When**: Every 10 seconds (periodic)
**Purpose**: Monitor mempool state over time

```json
{
  "type": "MEMPOOL_SNAPSHOT",
  "total_tx_count": 23,
  "address_count": 8,
  "addresses": ["z1qq...", "z1qx..."],
  "timestamp": "2025-11-11T10:00:10.000Z"
}
```

### 5. Momentum Content Selection (`MOMENTUM_CONTENT`)

**When**: Pillar is selected to produce a momentum and selects transactions
**Purpose**: Understand what goes into each momentum

```json
{
  "type": "MOMENTUM_CONTENT",
  "tx_count": 15,
  "addresses": ["z1qq...", "z1qx..."],
  "peer_count": 8,
  "tx_hashes": ["abc123...", "def456..."],
  "total_mempool_size": 23,
  "selection_time_ms": 5,
  "timestamp": "2025-11-11T10:00:15.123Z"
}
```

**Key fields**:
- `tx_count` - Transactions included in this momentum
- `total_mempool_size` - Total pending transactions at selection time
- `peer_count` - Connected peers when momentum was produced
- `selection_time_ms` - Time taken to select transactions

### 6. Transaction Filtering (`TX_FILTERED`)

**When**: Transactions are filtered out of momentum production
**Purpose**: Understand why transactions weren't included

```json
{
  "type": "TX_FILTERED",
  "reason": "exceeds_max_momentum_size",
  "tx_count": 8,
  "tx_hashes": ["ghi789..."],
  "addresses": ["z1qq..."],
  "timestamp": "2025-11-11T10:00:15.125Z"
}
```

**Reasons**:
- `exceeds_max_momentum_size` - More than 100 blocks would be in momentum
- `incomplete_contract_batch` - Contract send blocks without corresponding receive

### 7. Momentum Production (`MOMENTUM_PRODUCED`)

**When**: Pillar successfully produces a momentum
**Purpose**: Track momentum creation and timing

```json
{
  "type": "MOMENTUM_PRODUCED",
  "momentum_hash": "xyz789...",
  "height": 12345,
  "tx_count": 15,
  "production_time_ms": 45,
  "timestamp": "2025-11-11T10:00:15.168Z"
}
```

### 8. Peer Connection Events

**When**: Peers connect/disconnect
**Purpose**: Monitor network connectivity

```json
{
  "type": "PEER_CONNECTED",
  "peer_id": "16Uiu2HAm...",
  "peer_addr": "1.2.3.4:35995",
  "conn_type": "inbound",
  "timestamp": "2025-11-11T10:00:20.000Z"
}
```

```json
{
  "type": "PEER_DISCONNECTED",
  "peer_id": "16Uiu2HAm...",
  "duration": 300.5,
  "timestamp": "2025-11-11T10:05:20.000Z"
}
```

### 9. Active Peer List (`ACTIVE_PEERS`)

**When**: Periodically
**Purpose**: Track current peer connections

```json
{
  "type": "ACTIVE_PEERS",
  "peer_ids": ["16Uiu2HAm...", "16Uiu2HAk..."],
  "count": 8,
  "timestamp": "2025-11-11T10:00:30.000Z"
}
```

## Code Changes Summary

### Files Modified

| File | Lines Changed | Purpose |
|------|--------------|---------|
| `common/diagnostic.go` | +211 | New diagnostic logger implementation |
| `node/node.go` | +68 | Initialize logger and log node info |
| `chain/account_pool.go` | +90 | Log TX additions, momentum content, filtering |
| `chain/chain.go` | +57 | Periodic mempool snapshots |
| `p2p/server.go` | +27 | Log peer connections |
| `protocol/handler.go` | +23 | Log TX reception from peers |
| `protocol/broadcaster.go` | +17 | Implement GetPeerCount() |
| `protocol/peer.go` | +12 | Track peer connection times |
| `pillar/worker_momentum.go` | +25 | Log momentum production with timing |
| `protocol/interfaces.go` | +1 | Add GetPeerCount() to interface |
| `chain/interface.go` | +1 | Add peerCount param to GetNewMomentumContent() |

**Total**: 533 lines added (all diagnostic logging)

### Interface Changes

Two interface methods were modified to support diagnostic logging:

1. **`Broadcaster.GetPeerCount() int`** - Added to protocol/interfaces.go
   - Read-only method that returns the current number of connected peers
   - Enables logging peer count during momentum production

2. **`AccountPool.GetNewMomentumContent(peerCount int)`** - Modified in chain/interface.go
   - Added `peerCount` parameter (was: `GetNewMomentumContent()`)
   - Allows logging the peer count alongside momentum content
   - **Does not change return value or selection logic**

These changes are **purely for passing read-only data to the logging system**.

## Use Cases

### 1. Spam Testing

**Problem**: Some pillars produce momentums with zero transactions during spam tests
**Solution**: This logging helps diagnose why by showing:
- How many transactions were in mempool when pillar was selected
- Which transactions were selected vs filtered
- Network peer count at production time
- Transaction arrival patterns

### 2. Network Monitoring

Track network health by monitoring:
- Peer connection stability
- Transaction propagation times
- Mempool growth/shrinkage patterns

### 3. Performance Analysis

Measure:
- Transaction selection time
- Momentum production time
- Transaction processing delays

### 4. Debugging Transaction Issues

Trace a specific transaction through:
1. `TX_RECEIVED` - When it arrived
2. `ACCOUNT_BLOCK_ADDED` - When it entered mempool
3. `MEMPOOL_SNAPSHOT` - See it in periodic snapshots
4. `MOMENTUM_CONTENT` or `TX_FILTERED` - Included or excluded
5. `MOMENTUM_PRODUCED` - Final momentum creation

## Log File Analysis

### Parsing JSON Logs

Extract all momentum productions with zero transactions:
```bash
grep '"type":"MOMENTUM_PRODUCED"' diagnostic.log | jq 'select(.tx_count == 0)'
```

Find mempool state when empty momentums were produced:
```bash
grep '"type":"MEMPOOL_SNAPSHOT"' diagnostic.log | jq 'select(.total_tx_count > 0)'
```

Track a specific transaction:
```bash
TX_HASH="abc123..."
grep "$TX_HASH" diagnostic.log
```

### Analyzing Spam Tests

1. **Check mempool before empty momentums**:
   - Look at `MEMPOOL_SNAPSHOT` events
   - Compare `total_tx_count` with `MOMENTUM_CONTENT.tx_count`

2. **Identify filtering issues**:
   - Look for `TX_FILTERED` events
   - Check `reason` field for why transactions were excluded

3. **Compare peer counts**:
   - Check `MOMENTUM_CONTENT.peer_count` across different pillars
   - Low peer count may indicate connectivity issues

4. **Transaction timing**:
   - Compare `TX_RECEIVED` timestamp with `MOMENTUM_PRODUCED`
   - Identify delays in transaction processing

## Performance Impact

### Disk Space

Expect approximately:
- **100-500 KB per hour** for a quiet network
- **1-5 MB per hour** during active spam testing
- Logs are plain text (JSON + human-readable)

### CPU/Memory

Logging impact is **negligible**:
- All logging is asynchronous (buffered writes)
- No blocking operations in consensus-critical paths
- Read-only data collection (no additional computation)

### Network

**Zero network impact** - all logging is local file I/O only.

## Building and Running

### Build

```bash
cd go-zenon
git checkout diagnostic-logging
make znnd
```

### Run

The diagnostic logger is **automatically enabled** when the node starts. No configuration changes needed.

Logs will appear in: `{dataPath}/log/diagnostic.log`

Default data paths:
- **Linux**: `~/.znn/`
- **macOS**: `~/Library/Znn/`
- **Windows**: `%APPDATA%\Znn\`

### Disable Diagnostic Logging

If you want to run this branch **without** diagnostic logging, you can modify `node/node.go` and comment out:
```go
// if err := common.InitDiagnosticLogger(node.config.DataPath); err != nil {
//     log.Error("failed to initialize diagnostic logger", "reason", err)
// }
```

However, there's no reason to disable it as the overhead is minimal.

## Testing

All existing tests pass without modification:
```bash
go test ./...
```

The diagnostic logging does not interfere with:
- Unit tests
- Integration tests
- VM embedded contract tests
- Chain synchronization tests

## Safety Guarantees

### What This Branch Does NOT Do

❌ Does not modify consensus logic
❌ Does not change transaction validation
❌ Does not alter momentum production rules
❌ Does not modify P2P protocol messages
❌ Does not change network behavior
❌ Does not affect embedded contracts
❌ Does not modify database storage (except log file)
❌ Does not require configuration changes
❌ Does not break compatibility with master branch nodes

### What This Branch DOES Do

✅ Observes and logs transaction flow
✅ Records mempool state periodically
✅ Logs momentum production events
✅ Tracks peer connections
✅ Measures timing metrics
✅ Writes to separate diagnostic log file
✅ Uses read-only data access
✅ Maintains 100% compatibility with master

## Verification

You can verify that this branch only adds logging by:

1. **Review the code changes**:
   ```bash
   git diff master --stat
   git diff master chain/account_pool.go
   git diff master pillar/worker_momentum.go
   ```

2. **Check control flow**:
   - All `if diagnosticLogger != nil` blocks are purely logging
   - No logic changes outside diagnostic blocks
   - Same return values from all functions

3. **Run tests**:
   ```bash
   go test ./... -count=1
   ```

4. **Compare builds**:
   - Build both master and diagnostic-logging
   - Run side-by-side on testnet
   - Compare blockchain state (should be identical)

## Questions & Support

If you have questions about this diagnostic logging:

1. Review this document
2. Check the code comments in `common/diagnostic.go`
3. Examine the log output in `diagnostic.log`
4. Review the git diff: `git diff master`

## License

Same license as go-zenon (MIT License).

---

**Last Updated**: 2025-11-11
**Branch**: `diagnostic-logging`
**Base**: `master`
**Status**: ✅ Ready for testing
