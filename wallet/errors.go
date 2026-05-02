package wallet

import "github.com/pkg/errors"

// Sentinel errors returned by the wallet subsystem. They are grouped by
// the operation that surfaces them so callers can branch on the specific
// failure mode without parsing error messages.
var (
	// === KeyFile content errors ===

	// ErrKeyFileInvalidVersion is returned by [ReadKeyFile] when the
	// on-disk schema version does not match this build's expectation.
	ErrKeyFileInvalidVersion = errors.New("unable to read KeyFile. Invalid version")
	// ErrKeyFileInvalidCipher is returned by [ReadKeyFile] when the
	// keyfile's cipher name does not match this build's expectation.
	ErrKeyFileInvalidCipher = errors.New("unable to read KeyFile. Invalid cipherName")
	// ErrKeyFileInvalidKDF is returned by [ReadKeyFile] when the
	// keyfile's KDF identifier does not match this build's expectation.
	ErrKeyFileInvalidKDF = errors.New("unable to read KeyFile. Invalid key derivation function (KDF)")

	// === keyStore errors ===

	// ErrAddressNotFound is returned by [KeyStore.FindAddress] when no
	// derivation index up to maxSearchIndex matches the supplied address.
	ErrAddressNotFound = errors.New("the provided address could not be derived from the key store")
	// ErrWrongPassword is returned by [KeyFile.Decrypt] when the
	// AES-GCM authentication tag does not validate.
	ErrWrongPassword = errors.New("the key store could not be decrypted with the provided password")

	// === manager errors ===

	// ErrKeyStoreLocked is returned by [Manager.GetKeyStore] when the
	// requested keyfile is registered but not unlocked.
	ErrKeyStoreLocked = errors.New("the key store is locked")
	// ErrKeyStoreNotFound is returned by [Manager.GetKeyFile] /
	// [Manager.GetKeyStore] when no keyfile is registered for the
	// requested path.
	ErrKeyStoreNotFound = errors.New("the provided key store could not be found in the data directory")

	// === derivation errors ===

	// ErrInvalidPath is returned by [DeriveForPath] when the supplied
	// path does not parse as a hardened BIP-44 path.
	ErrInvalidPath = errors.New("invalid derivation path")
	// ErrNoPublicDerivation is returned by [DeriveForPath] when a
	// non-hardened segment is encountered. Ed25519 has no public-derivation
	// form.
	ErrNoPublicDerivation = errors.New("no public derivation for ed25519")
)
