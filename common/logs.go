package common

import (
	"bytes"
	"path/filepath"

	"github.com/inconshreveable/log15"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger is the project's logging facade. It is identical to
// [log15.Logger]; the alias exists so consumers import a single canonical
// logging interface from `common` rather than reaching into log15 directly.
type Logger interface {
	log15.Logger
}

// Per-subsystem loggers. Every package logs through one of these so log
// output can be filtered and routed by the `module` field. Use the
// per-subsystem variables — do not call `log15.New` ad hoc.
var (
	ChainLogger      = log15.New("module", "chain")
	ConsensusLogger  = log15.New("module", "consensus")
	NodeLogger       = log15.New("module", "node")
	P2PLogger        = log15.New("module", "p2p")
	PillarLogger     = log15.New("module", "pillar")
	ProtocolLogger   = log15.New("module", "handler")
	FetcherLogger    = ProtocolLogger.New("submodule", "fetcher")
	DownloaderLogger = ProtocolLogger.New("submodule", "downloader")
	RPCLogger        = log15.New("module", "rpc")
	VerifierLogger   = log15.New("module", "verifier")
	ZenonLogger      = log15.New("module", "zenon")
	VmLogger         = log15.New("module", "vm")
	SupervisorLogger = log15.New("module", "supervisor")
	EmbeddedLogger   = log15.New("module", "embedded")
	WalletLogger     = log15.New("module", "wallet")
)

// InitLogging wires the per-subsystem loggers to two rotating files:
// `<dataPath>/log/zenon.log` for events at logLevelStr or finer (but
// strictly below `error`), and `<dataPath>/log/error/zenon.error.log`
// for errors (note the additional `error/` subdirectory; see
// [runErrorLogHandler]). Invalid level strings fall back to Info.
func InitLogging(dataPath, logLevelStr string) {
	var logHandle []log15.Handler

	logDir := runLogDir(dataPath)
	logLevel, err := log15.LvlFromString(logLevelStr)
	if err != nil {
		logLevel = log15.LvlInfo
	}

	logHandle = append(logHandle, errorExcludeLvlFilterHandler(logLevel, runLogHandler(logDir)))
	logHandle = append(logHandle, log15.LvlFilterHandler(log15.LvlError, runErrorLogHandler(logDir)))

	log15.Root().SetHandler(log15.MultiHandler(
		logHandle...,
	))
}

// runLogDir returns the path to the log directory under dataPath.
func runLogDir(dataPath string) string {
	return filepath.Join(dataPath, "log")
}

// runLogHandler returns the log15 handler that writes the main rotating log.
func runLogHandler(logDir string) log15.Handler {
	filename := "zenon.log"
	logger := defaultLogger(filepath.Join(logDir, filename))
	return log15.StreamHandler(logger, log15.LogfmtFormat())
}

// runErrorLogHandler returns the log15 handler that writes errors only.
func runErrorLogHandler(logDir string) log15.Handler {
	filename := "zenon.error.log"
	logger := defaultLogger(filepath.Join(logDir, "error", filename))
	return log15.StreamHandler(logger, log15.LogfmtFormat())
}

// errorExcludeLvlFilterHandler returns a log15 filter that admits records
// at or below maxLvl — used to keep error records out of the main log
// (they go to the dedicated error log instead).
func errorExcludeLvlFilterHandler(maxLvl log15.Lvl, h log15.Handler) log15.Handler {
	return log15.FilterHandler(func(r *log15.Record) (ss bool) {
		return r.Lvl <= maxLvl
	}, h)
}

// defaultLogger returns the lumberjack rotating-file writer configured for
// node logs (100 MiB rotation, 14-day retention, no compression, UTC).
func defaultLogger(absFilePath string) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   absFilePath,
		MaxSize:    100,
		MaxBackups: 14,
		MaxAge:     14,
		Compress:   false,
		LocalTime:  false,
	}
}

// LogSaver is a log15.Format that captures every formatted record into an
// in-memory buffer instead of (or in addition to) writing to disk. Used by
// tests via [SaveLogs] to assert on log output. The format also overrides
// each record's timestamp with [Clock] so tests on a fake clock get
// deterministic output.
type LogSaver struct {
	format log15.Format
	buffer *bytes.Buffer
}

// Format formats r using the wrapped log15.Format after rewriting the
// record's timestamp from [Clock].
func (f LogSaver) Format(r *log15.Record) []byte {
	r.Time = Clock.Now()
	return f.format.Format(r)
}

// SaveLogs replaces log's handler with one that captures records into an
// in-memory buffer and returns an [Expecter] over the captured output.
// Used by tests to assert on what a subsystem logged.
func SaveLogs(log log15.Logger) *Expecter {
	logBuffer := new(bytes.Buffer)
	handler := &LogSaver{format: log15.LogfmtFormat(), buffer: logBuffer}
	log.SetHandler(log15.StreamHandler(logBuffer, handler))
	return LateCaller(func() (string, error) {
		return handler.buffer.String(), nil
	})
}
