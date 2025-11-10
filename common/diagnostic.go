package common

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DiagnosticLogger provides dual-format logging (JSON + human-readable) for diagnostic purposes
type DiagnosticLogger struct {
	file *os.File
	mu   sync.Mutex
}

var (
	diagnosticLogger *DiagnosticLogger
	diagnosticOnce   sync.Once
)

// InitDiagnosticLogger initializes the diagnostic logger
// Writes to dataPath/log/diagnostic.log with both JSON and human-readable formats
func InitDiagnosticLogger(dataPath string) error {
	var err error
	diagnosticOnce.Do(func() {
		logDir := filepath.Join(dataPath, "log")
		if err = os.MkdirAll(logDir, 0755); err != nil {
			return
		}

		logFile := filepath.Join(logDir, "diagnostic.log")
		file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return
		}

		diagnosticLogger = &DiagnosticLogger{
			file: file,
		}

		// Write initialization marker
		diagnosticLogger.logStartup()
	})
	return err
}

// GetDiagnosticLogger returns the singleton diagnostic logger instance
func GetDiagnosticLogger() *DiagnosticLogger {
	return diagnosticLogger
}

// Close closes the diagnostic log file
func (d *DiagnosticLogger) Close() error {
	if d == nil || d.file == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.file.Close()
}

// logStartup writes an initialization marker
func (d *DiagnosticLogger) logStartup() {
	d.log("DIAGNOSTIC_LOG_START", map[string]interface{}{
		"version": "1.0",
	})
}

// log writes both JSON and human-readable format to the diagnostic log
func (d *DiagnosticLogger) log(logType string, data map[string]interface{}) {
	if d == nil || d.file == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	data["timestamp"] = timestamp
	data["type"] = logType

	// Write JSON format
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	io.WriteString(d.file, string(jsonData)+"\n")

	// Write human-readable format
	readable := fmt.Sprintf("[%s] %s:", timestamp, logType)
	for k, v := range data {
		if k != "timestamp" && k != "type" {
			readable += fmt.Sprintf(" %s=%v", k, v)
		}
	}
	io.WriteString(d.file, readable+"\n")

	// Ensure data is written to disk
	d.file.Sync()
}

// LogNodeInfo logs node startup information
func (d *DiagnosticLogger) LogNodeInfo(nodeID, role, pillarName, producerAddr, listenAddr string, staticPeers, bootstrapPeers int, chainID int64, version string) {
	d.log("NODE_INFO", map[string]interface{}{
		"node_id":         nodeID,
		"role":            role,
		"pillar_name":     pillarName,
		"producer_addr":   producerAddr,
		"listen_addr":     listenAddr,
		"static_peers":    staticPeers,
		"bootstrap_peers": bootstrapPeers,
		"chain_id":        chainID,
		"version":         version,
	})
}

// LogTxBroadcast logs when a transaction is broadcast
func (d *DiagnosticLogger) LogTxBroadcast(txHash string, peerCount int, peerIDs []string) {
	d.log("TX_BROADCAST", map[string]interface{}{
		"tx_hash":    txHash,
		"peer_count": peerCount,
		"peer_ids":   peerIDs,
	})
}

// LogTxReceived logs when a transaction is received from a peer
func (d *DiagnosticLogger) LogTxReceived(txHash, fromPeer string) {
	d.log("TX_RECEIVED", map[string]interface{}{
		"tx_hash":   txHash,
		"from_peer": fromPeer,
	})
}

// LogPeerConnected logs when a peer connects
func (d *DiagnosticLogger) LogPeerConnected(peerID, peerAddr, connType string) {
	d.log("PEER_CONNECTED", map[string]interface{}{
		"peer_id":   peerID,
		"peer_addr": peerAddr,
		"conn_type": connType,
	})
}

// LogPeerDisconnected logs when a peer disconnects
func (d *DiagnosticLogger) LogPeerDisconnected(peerID string, duration float64) {
	d.log("PEER_DISCONNECTED", map[string]interface{}{
		"peer_id":  peerID,
		"duration": duration,
	})
}

// LogActivePeers logs the current active peer list
func (d *DiagnosticLogger) LogActivePeers(peerIDs []string, count int) {
	d.log("ACTIVE_PEERS", map[string]interface{}{
		"peer_ids": peerIDs,
		"count":    count,
	})
}

// LogAccountBlockAdded logs when an account block is added to the pool
func (d *DiagnosticLogger) LogAccountBlockAdded(txHash, address string, height uint64, source string) {
	d.log("ACCOUNT_BLOCK_ADDED", map[string]interface{}{
		"tx_hash": txHash,
		"address": address,
		"height":  height,
		"source":  source,
	})
}

// LogMomentumContent logs the content selected for momentum production
func (d *DiagnosticLogger) LogMomentumContent(txCount int, addresses []string, peerCount int) {
	d.log("MOMENTUM_CONTENT", map[string]interface{}{
		"tx_count":   txCount,
		"addresses":  addresses,
		"peer_count": peerCount,
	})
}

// LogMomentumProduced logs when a momentum is produced
func (d *DiagnosticLogger) LogMomentumProduced(momentumHash string, height uint64, txCount int) {
	d.log("MOMENTUM_PRODUCED", map[string]interface{}{
		"momentum_hash": momentumHash,
		"height":        height,
		"tx_count":      txCount,
	})
}
