// Package e2e is the end-to-end driver for a running go-zenon network.
//
// It builds, signs and publishes account blocks over JSON-RPC, using the
// repository's own wallet, block, ABI and proof-of-work packages, and reads
// the results back through the public RPC getters. It needs no SDK.
//
// The cases live in the *_test.go files behind the "e2e" build tag, so
// "go test ./..." never starts them. See docs/TESTING.md, procedure P9.
package e2e

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/tyler-smith/go-bip39"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/pow"
	"github.com/zenon-network/go-zenon/rpc/api"
	"github.com/zenon-network/go-zenon/rpc/server"
	"github.com/zenon-network/go-zenon/wallet"
)

// DevMnemonic is the committed devnet mnemonic (cmd/devnet-keygen). It holds
// no value outside a local devnet.
const DevMnemonic = "abstract affair idle position alien fluid board ordinary exist afraid chapter wood wood guide sun walnut crew perfect place firm poverty model side million"

// Derivation indexes of the devnet accounts (docker/devnet/README.md).
const (
	IdxProducer1 = 0 // pillar 1 producer; also the genesis spork address
	IdxOwner1    = 1 // pillar 1 owner; has fused plasma at genesis
	IdxOwner2    = 5
	IdxOwner3    = 7
	IdxFunder    = 3 // funded general account
)

// Contract-receive status values (vm/vm.go: resultSuccess, resultFail).
const (
	StatusSuccess uint64 = 1
	StatusFail    uint64 = 2
)

// Net is a connection to one network through one or more RPC endpoints.
// The first endpoint is used for publishing; all of them are used when a
// case asks whether every node agrees.
type Net struct {
	URLs    []string
	ChainID uint64

	clients []*server.Client
	ks      *wallet.KeyStore

	mu    sync.Mutex
	locks map[types.Address]*sync.Mutex
}

// Dial connects to every endpoint and derives the devnet key store.
func Dial(urls []string, chainID uint64) (*Net, error) {
	n := &Net{URLs: urls, ChainID: chainID, locks: map[types.Address]*sync.Mutex{}}
	for _, u := range urls {
		c, err := server.Dial(u)
		if err != nil {
			return nil, fmt.Errorf("dial %s: %w", u, err)
		}
		n.clients = append(n.clients, c)
	}
	entropy, err := bip39.EntropyFromMnemonic(DevMnemonic)
	if err != nil {
		return nil, err
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, err
	}
	n.ks = &wallet.KeyStore{Entropy: entropy, Seed: bip39.NewSeed(mnemonic, ""), Mnemonic: mnemonic}
	return n, nil
}

// Key returns the key pair at the given derivation index.
func (n *Net) Key(index uint32) *wallet.KeyPair {
	_, kp, err := n.ks.DeriveForIndexPath(index)
	if err != nil {
		panic(err)
	}
	return kp
}

// Call runs one RPC method on the first endpoint.
func (n *Net) Call(result interface{}, method string, args ...interface{}) error {
	return n.clients[0].Call(result, method, args...)
}

// CallOn runs one RPC method on endpoint i.
func (n *Net) CallOn(i int, result interface{}, method string, args ...interface{}) error {
	return n.clients[i].Call(result, method, args...)
}

// Frontier returns the frontier momentum of the first endpoint.
func (n *Net) Frontier() (*api.Momentum, error) {
	var m *api.Momentum
	if err := n.Call(&m, "ledger.getFrontierMomentum"); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("no frontier momentum")
	}
	return m, nil
}

// WaitHeight blocks until the first endpoint reaches the height.
func (n *Net) WaitHeight(height uint64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		m, err := n.Frontier()
		if err == nil && m.Height >= height {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("height %d not reached in %s (last error: %v)", height, timeout, err)
		}
		time.Sleep(time.Second)
	}
}

// Balance returns the confirmed balance of one token.
func (n *Net) Balance(addr types.Address, zts types.ZenonTokenStandard) (*big.Int, error) {
	var info *api.AccountInfo
	if err := n.Call(&info, "ledger.getAccountInfoByAddress", addr); err != nil {
		return nil, err
	}
	if info == nil || info.BalanceInfoMap[zts] == nil {
		return big.NewInt(0), nil
	}
	return info.BalanceInfoMap[zts].Balance, nil
}

func (n *Net) lock(addr types.Address) *sync.Mutex {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.locks[addr] == nil {
		n.locks[addr] = &sync.Mutex{}
	}
	return n.locks[addr]
}

type plasmaQuote struct {
	AvailablePlasma     uint64 `json:"availablePlasma"`
	BasePlasma          uint64 `json:"basePlasma"`
	RequiredDifficulty  uint64 `json:"requiredDifficulty"`
	RequiredFusedPlasma uint64 `json:"requiredFusedPlasma"`
}

type plasmaParam struct {
	Address   types.Address  `json:"address"`
	BlockType uint64         `json:"blockType"`
	ToAddress *types.Address `json:"toAddress"`
	Data      []byte         `json:"data"`
}

// Mutator changes a prepared block before it is hashed and signed (stage
// "fields"), or after signing (stage "signed"). Negative cases use it to
// build blocks that a correct client would never send.
type Mutator struct {
	Fields func(b *nom.AccountBlock) // before plasma, hash and signature
	Plasma func(b *nom.AccountBlock) // after the plasma fields are set, before hashing
	Signed func(b *nom.AccountBlock) // after signing; the hash is not recomputed
	// SkipPoW leaves the nonce zero even when the block states a difficulty.
	SkipPoW bool
}

// Prepare fills every field of the template the way a wallet does: version,
// chain identifier, position in the account chain, acknowledged momentum,
// plasma or proof of work, hash and signature.
func (n *Net) Prepare(kp *wallet.KeyPair, tmpl *nom.AccountBlock, mut *Mutator) (*nom.AccountBlock, error) {
	b := tmpl.Copy()
	b.Version = 1
	b.ChainIdentifier = n.ChainID
	b.Address = kp.Address
	if b.Amount == nil {
		b.Amount = big.NewInt(0)
	}
	if nom.IsReceiveBlock(b.BlockType) {
		b.Amount = big.NewInt(0)
		b.TokenStandard = types.ZeroTokenStandard
	}

	var prev *api.AccountBlock
	if err := n.Call(&prev, "ledger.getFrontierAccountBlock", kp.Address); err != nil {
		return nil, fmt.Errorf("frontier account block: %w", err)
	}
	if prev != nil {
		b.PreviousHash = prev.Hash
		b.Height = prev.Height + 1
	} else {
		b.PreviousHash = types.ZeroHash
		b.Height = 1
	}

	m, err := n.Frontier()
	if err != nil {
		return nil, err
	}
	b.MomentumAcknowledged = types.HashHeight{Hash: m.Hash, Height: m.Height}

	if mut != nil && mut.Fields != nil {
		mut.Fields(b)
	}

	param := plasmaParam{Address: b.Address, BlockType: b.BlockType, Data: b.Data}
	if nom.IsSendBlock(b.BlockType) {
		to := b.ToAddress
		param.ToAddress = &to
	}
	var q plasmaQuote
	if err := n.Call(&q, "embedded.plasma.getRequiredPoWForAccountBlock", param); err != nil {
		// The node refuses to price some invalid blocks.
		// A case built with a mutator publishes it anyway, without plasma,
		// so that the refusal it checks is the one of the publish path.
		if mut == nil {
			return nil, fmt.Errorf("plasma quote: %w", err)
		}
	}
	if q.RequiredDifficulty == 0 {
		b.FusedPlasma = q.RequiredFusedPlasma
	} else {
		b.FusedPlasma = q.AvailablePlasma
		b.Difficulty = q.RequiredDifficulty
	}
	if mut != nil && mut.Plasma != nil {
		mut.Plasma(b)
	}
	if b.Difficulty != 0 && (mut == nil || !mut.SkipPoW) {
		nonce := pow.GetPoWNonce(new(big.Int).SetUint64(b.Difficulty), pow.GetAccountBlockHash(b))
		copy(b.Nonce.Data[:], nonce)
	}

	b.Hash = b.ComputeHash()
	b.PublicKey = kp.Public
	b.Signature = kp.Sign(b.Hash.Bytes())
	if mut != nil && mut.Signed != nil {
		mut.Signed(b)
	}
	return b, nil
}

// Publish hands a block to the node as it is.
func (n *Net) Publish(b *nom.AccountBlock) error {
	return n.Call(nil, "ledger.publishRawTransaction", &api.AccountBlock{AccountBlock: *b})
}

// Send prepares and publishes a block. A quote can go stale when the
// frontier moves between the quote and the publish (dynamic plasma prices
// change per momentum), so a plasma refusal is retried on a fresh quote.
// Blocks with a mutator are never retried: their refusal is the result.
func (n *Net) Send(kp *wallet.KeyPair, tmpl *nom.AccountBlock, mut *Mutator) (*nom.AccountBlock, error) {
	l := n.lock(kp.Address)
	l.Lock()
	defer l.Unlock()
	for attempt := 0; ; attempt++ {
		b, err := n.Prepare(kp, tmpl, mut)
		if err != nil {
			return nil, err
		}
		err = n.Publish(b)
		if err == nil {
			return b, nil
		}
		stale := strings.Contains(err.Error(), "TotalPlasma") || strings.Contains(err.Error(), "price")
		if mut != nil || !stale || attempt >= 3 {
			return b, err
		}
		time.Sleep(time.Second)
	}
}

// Block reads one account block by hash from endpoint i.
func (n *Net) Block(i int, hash types.Hash) (*api.AccountBlock, error) {
	var b *api.AccountBlock
	err := n.CallOn(i, &b, "ledger.getAccountBlockByHash", hash)
	return b, err
}

// WaitConfirmed blocks until the block is in a momentum on the first
// endpoint and returns it as the node reports it.
func (n *Net) WaitConfirmed(hash types.Hash, timeout time.Duration) (*api.AccountBlock, error) {
	deadline := time.Now().Add(timeout)
	for {
		b, err := n.Block(0, hash)
		if err == nil && b != nil && b.ConfirmationDetail != nil {
			return b, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("block %s not confirmed in %s (last error: %v)", hash, timeout, err)
		}
		time.Sleep(time.Second)
	}
}

// Result is the outcome of a call to an embedded contract.
type Result struct {
	Send    *api.AccountBlock // the user's send block, confirmed
	Receive *api.AccountBlock // the contract's receive block, confirmed
	Status  uint64            // StatusSuccess or StatusFail
}

// OK reports whether the contract method executed without an error.
func (r *Result) OK() bool { return r.Status == StatusSuccess }

// CallContract sends data and an amount to an embedded contract and waits
// until both the send and the contract's receive are in momentums.
//
// A method can refuse in two places. A refusal while validating the send
// comes back as the error of this function, with the node's message. A
// refusal while the contract executes produces a receive block with
// StatusFail; the reason is then only in the node's log.
func (n *Net) CallContract(kp *wallet.KeyPair, to types.Address, data []byte, zts types.ZenonTokenStandard, amount *big.Int, timeout time.Duration) (*Result, error) {
	if amount == nil {
		amount = big.NewInt(0)
	}
	b, err := n.Send(kp, &nom.AccountBlock{
		BlockType:     nom.BlockTypeUserSend,
		ToAddress:     to,
		Data:          data,
		TokenStandard: zts,
		Amount:        amount,
	}, nil)
	if err != nil {
		return nil, err
	}
	return n.WaitContract(b.Hash, timeout)
}

// WaitContract waits for the contract receive that pairs with a send.
func (n *Net) WaitContract(sendHash types.Hash, timeout time.Duration) (*Result, error) {
	deadline := time.Now().Add(timeout)
	for {
		send, err := n.Block(0, sendHash)
		if err == nil && send != nil && send.ConfirmationDetail != nil && send.PairedAccountBlock != nil {
			recv, err2 := n.Block(0, send.PairedAccountBlock.Hash)
			if err2 == nil && recv != nil && recv.ConfirmationDetail != nil {
				if len(recv.Data) != 8 {
					return nil, fmt.Errorf("contract receive %s has %d data bytes, want 8", recv.Hash, len(recv.Data))
				}
				return &Result{Send: send, Receive: recv, Status: binary.BigEndian.Uint64(recv.Data)}, nil
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("contract receive for %s not confirmed in %s (last error: %v)", sendHash, timeout, err)
		}
		time.Sleep(time.Second)
	}
}

// Unreceived lists the first page of sends waiting for the address.
func (n *Net) Unreceived(addr types.Address) ([]*api.AccountBlock, error) {
	var list *api.AccountBlockList
	if err := n.Call(&list, "ledger.getUnreceivedBlocksByAddress", addr, 0, 50); err != nil {
		return nil, err
	}
	if list == nil {
		return nil, nil
	}
	return list.List, nil
}

// Receive publishes the receive block for one send.
func (n *Net) Receive(kp *wallet.KeyPair, from types.Hash, mut *Mutator) (*nom.AccountBlock, error) {
	return n.Send(kp, &nom.AccountBlock{BlockType: nom.BlockTypeUserReceive, FromBlockHash: from}, mut)
}

// ReceiveAll receives every pending send of the account until at least
// want of them have been received and none is left, then waits for the last
// receive to be confirmed.
func (n *Net) ReceiveAll(kp *wallet.KeyPair, want int, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	done := map[types.Hash]bool{}
	var last types.Hash
	for {
		pending, err := n.Unreceived(kp.Address)
		if err != nil {
			return len(done), err
		}
		fresh := 0
		for _, p := range pending {
			if done[p.Hash] {
				continue
			}
			b, err := n.Receive(kp, p.Hash, nil)
			if err != nil {
				return len(done), fmt.Errorf("receive %s: %w", p.Hash, err)
			}
			done[p.Hash] = true
			last = b.Hash
			fresh++
		}
		if fresh == 0 && len(done) >= want {
			if last == types.ZeroHash {
				return 0, nil
			}
			_, err := n.WaitConfirmed(last, time.Until(deadline))
			return len(done), err
		}
		if time.Now().After(deadline) {
			return len(done), fmt.Errorf("received %d of %d pending sends in %s", len(done), want, timeout)
		}
		time.Sleep(time.Second)
	}
}

// Agreement checks that every endpoint has the block in the same momentum.
func (n *Net) Agreement(hash types.Hash, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var first *api.AccountBlockConfirmationDetail
		problem := ""
		for i := range n.clients {
			b, err := n.Block(i, hash)
			if err != nil || b == nil || b.ConfirmationDetail == nil {
				problem = fmt.Sprintf("%s: block %s not confirmed (error: %v)", n.URLs[i], hash, err)
				break
			}
			if first == nil {
				first = b.ConfirmationDetail
			} else if !bytes.Equal(first.MomentumHash.Bytes(), b.ConfirmationDetail.MomentumHash.Bytes()) {
				return fmt.Errorf("%s confirms %s in momentum %s, %s in %s", n.URLs[0], hash, first.MomentumHash, n.URLs[i], b.ConfirmationDetail.MomentumHash)
			}
		}
		if problem == "" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s", problem)
		}
		time.Sleep(time.Second)
	}
}
