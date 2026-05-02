// Package wallet manages the on-disk encrypted keystore and the signing
// keys used by the pillar producer and the RPC layer.
//
// # Overview
//
// wallet is a thin, security-focused subsystem with three responsibilities:
//
//  1. Manage the on-disk wallet directory: discover [KeyFile]s, load and
//     parse them, persist new ones.
//  2. Decrypt and hold [KeyStore]s in memory after the user supplies the
//     correct password.
//  3. Derive Ed25519 [KeyPair]s from a BIP-39 seed via the canonical
//     Zenon derivation path (`m/44'/73404'/i'`).
//
// Encryption: BIP-39 entropy is sealed with AES-256-GCM under a key
// derived from the user's password by Argon2id. The cipher and KDF
// parameters are stamped on every [KeyFile]; readers reject mismatches up
// front.
//
// # Key Concepts
//
//   - KeyFile — on-disk encrypted wallet record. Discovered and tracked
//     by [Manager] from [Config.WalletDir].
//   - KeyStore — in-memory decrypted wallet (entropy, mnemonic, seed,
//     base address). Produced by [KeyFile.Decrypt] or
//     [keyStoreFromEntropy]. Always [KeyStore.Zero]ed when no longer
//     needed.
//   - KeyPair — Ed25519 keypair plus the derived [types.Address].
//     Returned by every derivation entry point and by
//     [KeyStore.FindAddress].
//   - Derivation path — `m/44'/73404'/i'`, fully hardened. Ed25519 has
//     no public-derivation form, so non-hardened segments return
//     [ErrNoPublicDerivation].
//   - Manager — orchestrates the whole thing: scans the wallet dir,
//     unlocks/locks keyfiles, hands out [KeyStore]s on demand.
//
// # Usage
//
// Boot:
//
//	mgr := wallet.New(&wallet.Config{WalletDir: "/path/to/wallet"})
//	if err := mgr.Start(); err != nil { /* handle */ }
//	defer mgr.Stop()
//
// Unlock and sign:
//
//	if err := mgr.Unlock(path, password); err != nil { /* handle */ }
//	ks, _ := mgr.GetKeyStore(path)
//	_, kp, _ := ks.DeriveForIndexPath(0)
//	sig := kp.Sign(message)
//
// # Concurrency
//
// [Manager] is not internally synchronized; callers that share an instance
// across goroutines must synchronize externally. [KeyStore.Zero] is racy
// against concurrent derivation — lock or stop the manager first.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/common/types] — the
//     [types.Address] keypairs derive to.
//   - [github.com/zenon-network/go-zenon/common/crypto] — supplies the
//     hash and Ed25519 primitives layered atop the standard library here.
//   - [github.com/zenon-network/go-zenon/pillar] — consumes a [KeyPair]
//     as its coinbase.
//   - [github.com/zenon-network/go-zenon/rpc/api] — uses [Manager] to
//     unlock and sign on behalf of users.
package wallet
