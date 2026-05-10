package wallet

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/zenon-network/go-zenon/common"
)

// DefaultMaxIndex is the fallback derivation-index ceiling applied when
// [Config.MaxSearchIndex] is left zero.
const (
	DefaultMaxIndex = 128
)

// Config configures a [Manager]: where to find key files on disk and a
// derivation-index ceiling.
//
// NOTE: MaxSearchIndex is currently dead config — [KeyStore.FindAddress]
// (keystore.go:90) uses the file-local maxSearchIndex constant (128),
// not this field. Setting MaxSearchIndex on a Config has no effect on
// search behavior. Kept on the struct for API stability.
type Config struct {
	WalletDir      string
	MaxSearchIndex uint32
}

// Manager is the wallet subsystem: it discovers key files in
// [Config.WalletDir], holds the encrypted [KeyFile]s in memory, and tracks
// which of them have been unlocked (decrypted into [KeyStore]s) by the user.
//
// Concurrency: the manager is not internally synchronized; callers that
// share an instance across goroutines must synchronize externally.
type Manager struct {
	config *Config
	log    common.Logger

	encrypted map[string]*KeyFile  // map from path to
	decrypted map[string]*KeyStore // map from path to
}

// New constructs a [Manager] from config. Returns nil if config is nil;
// fills DefaultMaxIndex when [Config.MaxSearchIndex] is left at zero.
func New(config *Config) *Manager {
	if config == nil {
		return nil
	}

	if config.MaxSearchIndex == 0 {
		config.MaxSearchIndex = DefaultMaxIndex
	}

	return &Manager{
		config:    config,
		encrypted: make(map[string]*KeyFile),
		decrypted: make(map[string]*KeyStore),
		log:       common.WalletLogger,
	}
}

// Start ensures [Config.WalletDir] exists and populates the encrypted
// keyfile index by scanning the directory.
func (m *Manager) Start() error {
	// ensure WalletDir exists
	if err := os.MkdirAll(m.config.WalletDir, 0700); err != nil {
		return err
	}
	m.log.Info("successfully ensured WalletDir exists", "wallet-dir-path", m.config.WalletDir)

	m.encrypted = make(map[string]*KeyFile)
	keyFiles, err := m.ListEntropyFilesInStandardDir()
	if err != nil {
		m.log.Error("wallet start err", "err", err)
		return err
	}
	for _, keyFile := range keyFiles {
		m.encrypted[keyFile.Path] = keyFile
	}
	return nil
}

// Stop zeros out every decrypted keystore and clears the indexes so secret
// material does not outlive the manager.
func (m *Manager) Stop() {
	for _, ks := range m.decrypted {
		ks.Zero()
	}
	m.decrypted = nil
	m.encrypted = nil
}

// MakePathAbsolut resolves path against [Config.WalletDir] when it is
// relative; absolute paths pass through unchanged.
func (m *Manager) MakePathAbsolut(path string) string {
	if filepath.IsAbs(path) {
		return path
	} else {
		return filepath.Join(m.config.WalletDir, path)
	}
}

// GetKeyFile returns the encrypted keyfile registered for path, or
// [ErrKeyStoreNotFound] if none is.
func (m *Manager) GetKeyFile(path string) (*KeyFile, error) {
	path = m.MakePathAbsolut(path)
	if kf, ok := m.encrypted[path]; !ok {
		return nil, ErrKeyStoreNotFound
	} else {
		return kf, nil
	}
}

// GetKeyStore returns the unlocked keystore for path, or
// [ErrKeyStoreNotFound] if the keyfile is unknown / [ErrKeyStoreLocked]
// if it is registered but not unlocked.
func (m *Manager) GetKeyStore(path string) (*KeyStore, error) {
	path = m.MakePathAbsolut(path)
	if _, ok := m.encrypted[path]; !ok {
		return nil, ErrKeyStoreNotFound
	} else if ks, ok := m.decrypted[path]; !ok {
		return nil, ErrKeyStoreLocked
	} else {
		return ks, nil
	}
}

// GetKeyFileAndDecrypt loads the keyfile at path and decrypts it with
// password. Does not register the resulting keystore on the manager —
// callers that want persistent unlock should use [Manager.Unlock].
func (m *Manager) GetKeyFileAndDecrypt(path, password string) (*KeyStore, error) {
	if kf, err := m.GetKeyFile(path); err != nil {
		return nil, err
	} else {
		return kf.Decrypt(password)
	}
}

// ListEntropyFilesInStandardDir reads them from the disk
func (m *Manager) ListEntropyFilesInStandardDir() ([]*KeyFile, error) {
	filePaths, err := os.ReadDir(m.config.WalletDir)
	if err != nil {
		return nil, err
	}

	files := make([]*KeyFile, 0)
	for _, file := range filePaths {
		if file.IsDir() || file.Type() != 0 {
			continue
		}
		fn := file.Name()
		if strings.HasPrefix(fn, ".") || strings.HasSuffix(fn, "~") {
			continue
		}
		absFilePath := filepath.Join(m.config.WalletDir, file.Name())
		keyFile, _ := ReadKeyFile(absFilePath)
		if keyFile != nil {
			files = append(files, keyFile)
		}
	}

	return files, nil
}

// Unlock decrypts the keyfile at path with password and registers both
// the keyfile (if not already known) and the resulting keystore on the
// manager. After this call [Manager.GetKeyStore] succeeds for path.
func (m *Manager) Unlock(path, password string) error {
	path = m.MakePathAbsolut(path)
	kf, err := m.GetKeyFile(path)
	if err != nil {
		return err
	}
	ks, err := kf.Decrypt(password)
	if err != nil {
		return err
	}

	m.encrypted[path] = kf
	m.decrypted[path] = ks
	return nil
}

// Lock zeros and forgets the decrypted keystore for path. Subsequent
// [Manager.GetKeyStore] calls will return [ErrKeyStoreLocked] until the
// caller unlocks again.
func (m *Manager) Lock(path string) {
	path = m.MakePathAbsolut(path)
	if ks, ok := m.decrypted[path]; ok {
		ks.Zero()
		m.decrypted[path] = nil
	}
}

// IsUnlocked reports whether the keystore at path is currently unlocked.
// Returns [ErrKeyStoreNotFound] if path is not a known keyfile.
func (m *Manager) IsUnlocked(path string) (bool, error) {
	path = m.MakePathAbsolut(path)
	if _, ok := m.encrypted[path]; !ok {
		return false, ErrKeyStoreNotFound
	}
	_, ok := m.decrypted[path]
	return ok, nil
}
