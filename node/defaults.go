package node

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"

	"github.com/zenon-network/go-zenon/p2p"
)

// DefaultWalletDir is the wallet sub-directory under DataPath when
// WalletPath is left unset.
const (
	DefaultWalletDir = "wallet"
)

// DefaultNodeConfig is the baseline node configuration used by the
// CLI when the operator does not provide a config.json. Defaults to
// a non-producing node listening on Alphanet ports with HTTP / WS
// RPC enabled and CORS / WSOrigins fully open ("*").
var DefaultNodeConfig = Config{
	DataPath: DefaultDataDir(),

	Name: p2p.DefaultNodeName,

	LogLevel: "info",

	RPC: RPCConfig{
		HTTPPort:   p2p.DefaultHTTPPort,
		HTTPHost:   "0.0.0.0",
		EnableHTTP: true,
		WSPort:     p2p.DefaultWSPort,
		WSHost:     "0.0.0.0",
		EnableWS:   true,

		HTTPCors:  []string{"*"},
		WSOrigins: []string{"*"},
	},
	Net: NetConfig{
		ListenHost:        p2p.DefaultListenHost,
		ListenPort:        p2p.DefaultListenPort,
		MinPeers:          p2p.DefaultMinPeers,
		MinConnectedPeers: p2p.DefaultMinConnectedPeers,
		MaxPeers:          p2p.DefaultMaxPeers,
		MaxPendingPeers:   p2p.DefaultMaxPendingPeers,
		Seeders:           p2p.DefaultSeeders,
	},
}

// DefaultDataDir is the default data directory to use for the databases and other persistence requirements.
func DefaultDataDir() string {
	// Try to place the data folder in the user's home dir
	home := homeDir()
	if home != "" {
		switch runtime.GOOS {
		case "darwin":
			return filepath.Join(home, "Library", "znn")
		case "windows":
			// We used to put everything in %HOME%\AppData\Roaming, but this caused
			// problems with non-typical setups. If this fallback location exists and
			// is non-empty, use it, otherwise DTRT and check %LOCALAPPDATA%.
			fallback := filepath.Join(home, "AppData", "Roaming", "znn")
			appdata := windowsAppData()
			if appdata == "" || isNonEmptyDir(fallback) {
				return fallback
			}
			return filepath.Join(appdata, "znn")
		default:
			return filepath.Join(home, ".znn")
		}
	}
	// As we cannot guess a stable location, return empty and handle later
	return ""
}

// windowsAppData returns %LOCALAPPDATA%. Panics if the variable is
// unset — Windows XP and below are unsupported.
func windowsAppData() string {
	v := os.Getenv("LOCALAPPDATA")
	if v == "" {
		// Windows XP and below don't have LocalAppData. Crash here because
		// we don't support Windows XP and undefining the variable will cause
		// other issues.
		panic("environment variable LocalAppData is undefined")
	}
	return v
}

// isNonEmptyDir reports whether dir exists and contains at least
// one entry — used by the Windows fallback path resolver.
func isNonEmptyDir(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	names, _ := f.Readdir(1)
	f.Close()
	return len(names) > 0
}

// homeDir returns the user's home directory: $HOME if set, otherwise
// the OS-reported user record's HomeDir, otherwise an empty string.
func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}

// ReplaceHomeVariable expands a leading `~` in path to the user's
// home directory. Returns "" for empty input.
func ReplaceHomeVariable(path string) string {
	if len(path) == 0 {
		return ""
	}
	if path[0] == '~' {
		return filepath.Join(homeDir(), path[1:])
	}
	return path
}
