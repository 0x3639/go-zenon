package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tyler-smith/go-bip39"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/pow"
	rpcapi "github.com/zenon-network/go-zenon/rpc/api"
	embeddedapi "github.com/zenon-network/go-zenon/rpc/api/embedded"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/wallet"
)

const (
	devMnemonic = "abstract affair idle position alien fluid board ordinary exist afraid chapter wood wood guide sun walnut crew perfect place firm poverty model side million"

	defaultRPCURL  = "http://localhost:35991"
	operatorIndex  = 1
	maxLabelLength = 12
)

type pillarOwner struct {
	Name  string
	Index uint32
}

var devnetPillars = []pillarOwner{
	{Name: "dev1", Index: 1},
	{Name: "dev2", Index: 5},
	{Name: "dev3", Index: 7},
	{Name: "dev4", Index: 11},
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (e *rpcError) Error() string {
	if len(e.Data) == 0 {
		return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("rpc error %d: %s: %s", e.Code, e.Message, string(e.Data))
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcClient struct {
	url    string
	client *http.Client
	nextID uint64
}

func newRPCClient(url string) *rpcClient {
	return &rpcClient{
		url:    url,
		client: &http.Client{Timeout: 2 * time.Minute},
	}
}

func (c *rpcClient) call(ctx context.Context, method string, params []any, result any) error {
	c.nextID++
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      c.nextID,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var decoded rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return err
	}
	if decoded.Error != nil {
		return decoded.Error
	}
	if result == nil || len(decoded.Result) == 0 || string(decoded.Result) == "null" {
		return nil
	}
	return json.Unmarshal(decoded.Result, result)
}

type frontierMomentum struct {
	Hash      types.Hash `json:"hash"`
	Height    uint64     `json:"height"`
	Timestamp int64      `json:"timestamp"`
}

func (c *rpcClient) frontierMomentum(ctx context.Context) (*frontierMomentum, error) {
	var momentum frontierMomentum
	if err := c.call(ctx, "ledger.getFrontierMomentum", []any{}, &momentum); err != nil {
		return nil, err
	}
	return &momentum, nil
}

func (c *rpcClient) frontierAccountBlock(ctx context.Context, address types.Address) (*rpcapi.AccountBlock, error) {
	var raw json.RawMessage
	if err := c.call(ctx, "ledger.getFrontierAccountBlock", []any{address}, &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var block rpcapi.AccountBlock
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, err
	}
	if block.Hash.IsZero() {
		return nil, nil
	}
	return &block, nil
}

func (c *rpcClient) accountBlockByHash(ctx context.Context, hash types.Hash) (*rpcapi.AccountBlock, error) {
	var raw json.RawMessage
	if err := c.call(ctx, "ledger.getAccountBlockByHash", []any{hash}, &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var block rpcapi.AccountBlock
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, err
	}
	if block.Hash.IsZero() {
		return nil, nil
	}
	return &block, nil
}

type requiredPoWParam struct {
	SelfAddr  types.Address  `json:"address"`
	BlockType uint64         `json:"blockType"`
	ToAddr    *types.Address `json:"toAddress"`
	Data      []byte         `json:"data"`
}

type requiredPoWResult struct {
	AvailablePlasma    uint64 `json:"availablePlasma"`
	BasePlasma         uint64 `json:"basePlasma"`
	RequiredDifficulty uint64 `json:"requiredDifficulty"`
}

func (c *rpcClient) requiredPoW(ctx context.Context, address, to types.Address, data []byte) (*requiredPoWResult, error) {
	param := requiredPoWParam{
		SelfAddr:  address,
		BlockType: nom.BlockTypeUserSend,
		ToAddr:    &to,
		Data:      data,
	}
	var result requiredPoWResult
	if err := c.call(ctx, "embedded.plasma.getRequiredPoWForAccountBlock", []any{param}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *rpcClient) publishRawTransaction(ctx context.Context, block *nom.AccountBlock) error {
	return c.call(ctx, "ledger.publishRawTransaction", []any{&rpcapi.AccountBlock{AccountBlock: *block}}, nil)
}

func (c *rpcClient) getAction(ctx context.Context, id types.Hash) (*embeddedapi.Action, error) {
	var action embeddedapi.Action
	if err := c.call(ctx, "embedded.governance.getActionById", []any{id}, &action); err != nil {
		return nil, err
	}
	return &action, nil
}

func (c *rpcClient) getSporks(ctx context.Context) (*embeddedapi.SporkList, error) {
	var list embeddedapi.SporkList
	if err := c.call(ctx, "embedded.spork.getAll", []any{uint32(0), uint32(100)}, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

func (c *rpcClient) getLiquidityInfo(ctx context.Context) (*definition.LiquidityInfo, error) {
	var info definition.LiquidityInfo
	if err := c.call(ctx, "embedded.liquidity.getLiquidityInfo", []any{}, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *rpcClient) getPillars(ctx context.Context) (*embeddedapi.PillarInfoList, error) {
	var list embeddedapi.PillarInfoList
	if err := c.call(ctx, "embedded.pillar.getAll", []any{uint32(0), uint32(100)}, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

func (c *rpcClient) unreceivedCount(ctx context.Context, address types.Address) (int, error) {
	var list rpcapi.AccountBlockList
	if err := c.call(ctx, "ledger.getUnreceivedBlocksByAddress", []any{address, uint32(0), uint32(10)}, &list); err != nil {
		return 0, err
	}
	return list.Count, nil
}

type scenario struct {
	rpc     *rpcClient
	keys    map[uint32]*wallet.KeyPair
	timeout time.Duration
}

func (s *scenario) key(index uint32) *wallet.KeyPair {
	return s.keys[index]
}

func (s *scenario) send(ctx context.Context, key *wallet.KeyPair, to types.Address, token types.ZenonTokenStandard, amount *big.Int, data []byte) (*nom.AccountBlock, error) {
	momentum, err := s.rpc.frontierMomentum(ctx)
	if err != nil {
		return nil, err
	}

	previousHash := types.ZeroHash
	height := uint64(1)
	frontier, err := s.rpc.frontierAccountBlock(ctx, key.Address)
	if err != nil {
		return nil, err
	}
	if frontier != nil {
		previousHash = frontier.Hash
		height = frontier.Height + 1
	}

	required, err := s.rpc.requiredPoW(ctx, key.Address, to, data)
	if err != nil {
		return nil, err
	}

	block := &nom.AccountBlock{
		Version:              1,
		ChainIdentifier:      types.DevnetChainIdentifier,
		BlockType:            nom.BlockTypeUserSend,
		PreviousHash:         previousHash,
		Height:               height,
		MomentumAcknowledged: types.HashHeight{Hash: momentum.Hash, Height: momentum.Height},
		Address:              key.Address,
		ToAddress:            to,
		Amount:               new(big.Int).Set(amount),
		TokenStandard:        token,
		Data:                 data,
		FusedPlasma:          fusedPlasmaFor(required),
		Difficulty:           required.RequiredDifficulty,
	}
	if block.Difficulty > 0 {
		nonce := pow.GetPoWNonce(new(big.Int).SetUint64(block.Difficulty), pow.GetAccountBlockHash(block))
		block.Nonce = nom.DeSerializeNonce(nonce)
	}

	block.Hash = block.ComputeHash()
	block.PublicKey = key.Public
	block.Signature = key.Sign(block.Hash.Bytes())

	if err := s.rpc.publishRawTransaction(ctx, block); err != nil {
		return nil, err
	}
	if err := s.waitForAccountBlock(ctx, block.Hash); err != nil {
		return nil, err
	}
	return block, nil
}

func (s *scenario) expectRejected(ctx context.Context, key *wallet.KeyPair, to types.Address, token types.ZenonTokenStandard, amount *big.Int, data []byte, want string) error {
	momentum, err := s.rpc.frontierMomentum(ctx)
	if err != nil {
		return err
	}

	previousHash := types.ZeroHash
	height := uint64(1)
	frontier, err := s.rpc.frontierAccountBlock(ctx, key.Address)
	if err != nil {
		return err
	}
	if frontier != nil {
		previousHash = frontier.Hash
		height = frontier.Height + 1
	}

	required, err := s.rpc.requiredPoW(ctx, key.Address, to, data)
	if err != nil {
		return err
	}

	block := &nom.AccountBlock{
		Version:              1,
		ChainIdentifier:      types.DevnetChainIdentifier,
		BlockType:            nom.BlockTypeUserSend,
		PreviousHash:         previousHash,
		Height:               height,
		MomentumAcknowledged: types.HashHeight{Hash: momentum.Hash, Height: momentum.Height},
		Address:              key.Address,
		ToAddress:            to,
		Amount:               new(big.Int).Set(amount),
		TokenStandard:        token,
		Data:                 data,
		FusedPlasma:          fusedPlasmaFor(required),
		Difficulty:           required.RequiredDifficulty,
	}
	if block.Difficulty > 0 {
		nonce := pow.GetPoWNonce(new(big.Int).SetUint64(block.Difficulty), pow.GetAccountBlockHash(block))
		block.Nonce = nom.DeSerializeNonce(nonce)
	}
	block.Hash = block.ComputeHash()
	block.PublicKey = key.Public
	block.Signature = key.Sign(block.Hash.Bytes())

	err = s.rpc.publishRawTransaction(ctx, block)
	if err == nil {
		return fmt.Errorf("expected transaction to be rejected with %q", want)
	}
	if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
		return fmt.Errorf("expected rejection containing %q, got %v", want, err)
	}
	return nil
}

func (s *scenario) propose(ctx context.Context, label, description string, destination types.Address, targetData []byte) (types.Hash, error) {
	data := definition.ABIGovernance.PackMethodPanic(
		definition.ProposeActionMethodName,
		label,
		description,
		"https://zenon.network",
		destination,
		base64.StdEncoding.EncodeToString(targetData),
	)
	block, err := s.send(ctx, s.key(operatorIndex), types.GovernanceContract, types.ZnnTokenStandard, big.NewInt(constants.Decimals), data)
	if err != nil {
		return types.ZeroHash, err
	}
	if _, err := s.waitAction(ctx, block.Hash); err != nil {
		return types.ZeroHash, err
	}
	return block.Hash, nil
}

func (s *scenario) vote(ctx context.Context, pillar pillarOwner, actionID types.Hash, vote uint8) error {
	data := definition.ABIGovernance.PackMethodPanic(definition.VoteByNameMethodName, actionID, pillar.Name, vote)
	_, err := s.send(ctx, s.key(pillar.Index), types.GovernanceContract, types.ZnnTokenStandard, big.NewInt(0), data)
	if err != nil {
		return err
	}
	return nil
}

func (s *scenario) execute(ctx context.Context, actionID types.Hash) error {
	data := definition.ABIGovernance.PackMethodPanic(definition.ExecuteActionMethodName, actionID)
	_, err := s.send(ctx, s.key(operatorIndex), types.GovernanceContract, types.ZnnTokenStandard, big.NewInt(0), data)
	if err != nil {
		return err
	}
	return nil
}

func (s *scenario) waitForAccountBlock(ctx context.Context, hash types.Hash) error {
	return waitUntil(ctx, s.timeout, 2*time.Second, func() (bool, error) {
		block, err := s.rpc.accountBlockByHash(ctx, hash)
		if err != nil {
			return false, nil
		}
		return block != nil, nil
	})
}

func (s *scenario) waitFrontierHeight(ctx context.Context, minHeight uint64) error {
	return waitUntil(ctx, s.timeout, 2*time.Second, func() (bool, error) {
		momentum, err := s.rpc.frontierMomentum(ctx)
		if err != nil {
			return false, nil
		}
		return momentum.Height >= minHeight, nil
	})
}

func (s *scenario) waitAction(ctx context.Context, id types.Hash) (*embeddedapi.Action, error) {
	var action *embeddedapi.Action
	err := waitUntil(ctx, s.timeout, 2*time.Second, func() (bool, error) {
		var err error
		action, err = s.rpc.getAction(ctx, id)
		if err != nil {
			return false, nil
		}
		return true, nil
	})
	return action, err
}

func (s *scenario) waitActionVotes(ctx context.Context, id types.Hash, yes, no uint32) error {
	return waitUntil(ctx, s.timeout, 2*time.Second, func() (bool, error) {
		action, err := s.rpc.getAction(ctx, id)
		if err != nil {
			return false, nil
		}
		return action.Votes.Yes == yes && action.Votes.No == no, nil
	})
}

func (s *scenario) waitActionRound(ctx context.Context, id types.Hash, round uint8, yes, no uint32) (*embeddedapi.Action, error) {
	var action *embeddedapi.Action
	err := waitUntil(ctx, s.timeout, 2*time.Second, func() (bool, error) {
		var err error
		action, err = s.rpc.getAction(ctx, id)
		if err != nil {
			return false, nil
		}
		return action.Round == round && action.Votes.Yes == yes && action.Votes.No == no, nil
	})
	return action, err
}

func (s *scenario) waitActionExpired(ctx context.Context, id types.Hash) error {
	return waitUntil(ctx, s.timeout, 2*time.Second, func() (bool, error) {
		action, err := s.rpc.getAction(ctx, id)
		if err != nil {
			return false, nil
		}
		return action.Expired, nil
	})
}

func (s *scenario) waitActionExecuted(ctx context.Context, id types.Hash) error {
	return waitUntil(ctx, s.timeout, 2*time.Second, func() (bool, error) {
		action, err := s.rpc.getAction(ctx, id)
		if err != nil {
			return false, nil
		}
		return action.Executed, nil
	})
}

func (s *scenario) waitActionStatus(ctx context.Context, id types.Hash, status uint8, yes, no uint32) (*embeddedapi.Action, error) {
	var action *embeddedapi.Action
	err := waitUntil(ctx, s.timeout, 2*time.Second, func() (bool, error) {
		var err error
		action, err = s.rpc.getAction(ctx, id)
		if err != nil {
			return false, nil
		}
		return action.Status == status && action.Votes.Yes == yes && action.Votes.No == no, nil
	})
	return action, err
}

func (s *scenario) waitLiquidityHalted(ctx context.Context, halted bool) error {
	return waitUntil(ctx, s.timeout, 2*time.Second, func() (bool, error) {
		info, err := s.rpc.getLiquidityInfo(ctx)
		if err != nil {
			return false, nil
		}
		return info.IsHalted == halted, nil
	})
}

func (s *scenario) waitSpork(ctx context.Context, name string) (*definition.Spork, error) {
	var spork *definition.Spork
	err := waitUntil(ctx, s.timeout, 2*time.Second, func() (bool, error) {
		var ok bool
		var err error
		spork, ok, err = s.sporkByName(ctx, name)
		if err != nil {
			return false, nil
		}
		return ok, nil
	})
	return spork, err
}

func (s *scenario) waitNoUnreceived(ctx context.Context, address types.Address) error {
	return waitUntil(ctx, s.timeout, 2*time.Second, func() (bool, error) {
		count, err := s.rpc.unreceivedCount(ctx, address)
		if err != nil {
			return false, nil
		}
		return count == 0, nil
	})
}

func (s *scenario) requirePillarCount(ctx context.Context, want uint32) error {
	pillars, err := s.rpc.getPillars(ctx)
	if err != nil {
		return err
	}
	if pillars.Count != want {
		return fmt.Errorf("devnet has %d active pillars, want %d", pillars.Count, want)
	}
	return nil
}

func (s *scenario) requireGovernanceActive(ctx context.Context) error {
	sporks, err := s.rpc.getSporks(ctx)
	if err != nil {
		return err
	}
	for _, spork := range sporks.List {
		if spork.Id == types.GovernanceSpork.SporkId && spork.Activated {
			return nil
		}
	}
	return fmt.Errorf("governance spork %s is not active on this devnet", types.GovernanceSpork.SporkId)
}

func (s *scenario) sporkByName(ctx context.Context, name string) (*definition.Spork, bool, error) {
	sporks, err := s.rpc.getSporks(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, spork := range sporks.List {
		if spork.Name == name {
			return spork, true, nil
		}
	}
	return nil, false, nil
}

func (s *scenario) sporkExists(ctx context.Context, name string) (bool, error) {
	_, ok, err := s.sporkByName(ctx, name)
	return ok, err
}

func (s *scenario) voteCurrentRound(ctx context.Context, pillar pillarOwner, actionID types.Hash, vote uint8) error {
	action, err := s.rpc.getAction(ctx, actionID)
	if err != nil {
		return err
	}
	return s.vote(ctx, pillar, action.CurrentVoteId, vote)
}

func ensureNotExecuted(action *embeddedapi.Action, reason string) error {
	if action.Executed || action.Status == constants.ActionStatusApproved {
		return fmt.Errorf("%s: action unexpectedly executed", reason)
	}
	return nil
}

func (s *scenario) runType1Ratchet(ctx context.Context, label string) error {
	sporkName := "spork-" + label
	type1ID, err := s.propose(
		ctx,
		"t1-"+label,
		"type1 spork create ratchet check",
		types.SporkContract,
		definition.ABISpork.PackMethodPanic(definition.SporkCreateMethodName, sporkName, "created by devnet governance ratchet scenario"),
	)
	if err != nil {
		return err
	}
	action, err := s.waitActionRound(ctx, type1ID, 0, 0, 0)
	if err != nil {
		return err
	}
	if action.Type != constants.Type1Action || action.ActivePillarThreshold != 66 || action.DirectionalThreshold != 50 {
		return fmt.Errorf("round 0 Type1 schedule = type %d, active %d, directional %d", action.Type, action.ActivePillarThreshold, action.DirectionalThreshold)
	}
	fmt.Printf("PASS proposed Type1 action %s at round 0 with 66%% active threshold\n", type1ID)

	for _, pillar := range devnetPillars[:2] {
		if err := s.voteCurrentRound(ctx, pillar, type1ID, definition.VoteYes); err != nil {
			return err
		}
	}
	if err := s.waitActionVotes(ctx, type1ID, 2, 0); err != nil {
		return err
	}
	if err := s.waitActionExpired(ctx, type1ID); err != nil {
		return err
	}
	if err := s.execute(ctx, type1ID); err != nil {
		return err
	}
	action, err = s.waitActionRound(ctx, type1ID, 1, 0, 0)
	if err != nil {
		return err
	}
	if err := ensureNotExecuted(action, "Type1 round 0 with 2 of 4 Yes votes"); err != nil {
		return err
	}
	exists, err := s.sporkExists(ctx, sporkName)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("Type1 target spork was created in round 0 with only 2 of 4 yes votes")
	}
	fmt.Println("PASS Type1 round 0 ratchets instead of executing with 2 of 4 Yes votes")
	if action.ActivePillarThreshold != 55 || action.DirectionalThreshold != 55 {
		return fmt.Errorf("round 1 Type1 schedule = active %d, directional %d", action.ActivePillarThreshold, action.DirectionalThreshold)
	}
	fmt.Println("PASS Type1 action ratcheted to round 1 with 55% active / 55% directional thresholds")

	for _, pillar := range devnetPillars[:2] {
		if err := s.voteCurrentRound(ctx, pillar, type1ID, definition.VoteYes); err != nil {
			return err
		}
	}
	if err := s.waitActionVotes(ctx, type1ID, 2, 0); err != nil {
		return err
	}
	if err := s.waitActionExpired(ctx, type1ID); err != nil {
		return err
	}
	if err := s.execute(ctx, type1ID); err != nil {
		return err
	}
	action, err = s.waitActionRound(ctx, type1ID, 2, 0, 0)
	if err != nil {
		return err
	}
	if action.ActivePillarThreshold != 45 || action.DirectionalThreshold != 60 {
		return fmt.Errorf("round 2 Type1 schedule = active %d, directional %d", action.ActivePillarThreshold, action.DirectionalThreshold)
	}
	fmt.Println("PASS Type1 action ratcheted to round 2 with 45% active / 60% directional thresholds")

	for _, pillar := range devnetPillars[:2] {
		if err := s.voteCurrentRound(ctx, pillar, type1ID, definition.VoteYes); err != nil {
			return err
		}
	}
	if err := s.waitActionVotes(ctx, type1ID, 2, 0); err != nil {
		return err
	}
	if err := s.waitActionExpired(ctx, type1ID); err != nil {
		return err
	}
	if err := s.execute(ctx, type1ID); err != nil {
		return err
	}
	if err := s.waitActionExecuted(ctx, type1ID); err != nil {
		return err
	}
	createdSpork, err := s.waitSpork(ctx, sporkName)
	if err != nil {
		return err
	}
	if createdSpork == nil {
		return fmt.Errorf("Type1 target spork %q was not created", sporkName)
	}
	fmt.Println("PASS Type1 action executes with 2 of 4 Yes votes only after ratcheting to round 2")
	return nil
}

func (s *scenario) runType1NoVoteExpiry(ctx context.Context, label string) error {
	sporkName := "novote-" + label
	type1ID, err := s.propose(
		ctx,
		"nv-"+label,
		"type1 no-vote expiry check",
		types.SporkContract,
		definition.ABISpork.PackMethodPanic(definition.SporkCreateMethodName, sporkName, "should not be created without governance votes"),
	)
	if err != nil {
		return err
	}
	action, err := s.waitActionRound(ctx, type1ID, 0, 0, 0)
	if err != nil {
		return err
	}
	if action.Type != constants.Type1Action || action.ActivePillarThreshold != 66 || action.DirectionalThreshold != 50 {
		return fmt.Errorf("no-vote Type1 round 0 schedule = type %d, active %d, directional %d", action.Type, action.ActivePillarThreshold, action.DirectionalThreshold)
	}
	fmt.Printf("PASS proposed no-vote Type1 action %s at round 0\n", type1ID)

	expectedActiveThresholds := []uint32{55, 45, 40}
	expectedDirectionalThresholds := []uint32{55, 60, 66}
	for nextRound := uint8(1); nextRound <= 3; nextRound++ {
		if err := s.waitActionExpired(ctx, type1ID); err != nil {
			return err
		}
		if err := s.execute(ctx, type1ID); err != nil {
			return err
		}
		action, err = s.waitActionRound(ctx, type1ID, nextRound, 0, 0)
		if err != nil {
			return err
		}
		if err := ensureNotExecuted(action, fmt.Sprintf("Type1 no-vote round %d", nextRound-1)); err != nil {
			return err
		}
		active := expectedActiveThresholds[nextRound-1]
		directional := expectedDirectionalThresholds[nextRound-1]
		if action.ActivePillarThreshold != active || action.DirectionalThreshold != directional {
			return fmt.Errorf("no-vote Type1 round %d schedule = active %d, directional %d", action.Round, action.ActivePillarThreshold, action.DirectionalThreshold)
		}
		fmt.Printf("PASS no-vote Type1 action ratcheted to round %d with zero votes\n", nextRound)
	}

	if err := s.waitActionExpired(ctx, type1ID); err != nil {
		return err
	}
	if err := s.execute(ctx, type1ID); err != nil {
		return err
	}
	action, err = s.waitActionStatus(ctx, type1ID, constants.ActionStatusNoDecision, 0, 0)
	if err != nil {
		return err
	}
	if action.Executed {
		return errors.New("no-vote Type1 action executed instead of closing with no decision")
	}
	exists, err := s.sporkExists(ctx, sporkName)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("no-vote Type1 target spork was created")
	}
	fmt.Println("PASS no-vote Type1 action expires through all ratchet rounds and closes with no decision")
	return nil
}

func (s *scenario) runType2Ratchet(ctx context.Context, label string) error {
	liquidityBefore, err := s.rpc.getLiquidityInfo(ctx)
	if err != nil {
		return err
	}
	targetHalted := !liquidityBefore.IsHalted
	type2ID, err := s.propose(
		ctx,
		"t2-"+label,
		"type2 liquidity ratchet check",
		types.LiquidityContract,
		definition.ABILiquidity.PackMethodPanic(definition.SetIsHaltedMethodName, targetHalted),
	)
	if err != nil {
		return err
	}
	action, err := s.waitActionRound(ctx, type2ID, 0, 0, 0)
	if err != nil {
		return err
	}
	if action.Type != constants.Type2Action || action.ActivePillarThreshold != 50 || action.DirectionalThreshold != 50 {
		return fmt.Errorf("round 0 Type2 schedule = type %d, active %d, directional %d", action.Type, action.ActivePillarThreshold, action.DirectionalThreshold)
	}
	fmt.Printf("PASS proposed Type2 action %s at round 0 with 50%% active threshold\n", type2ID)

	for _, pillar := range devnetPillars[:2] {
		if err := s.voteCurrentRound(ctx, pillar, type2ID, definition.VoteYes); err != nil {
			return err
		}
	}
	if err := s.waitActionVotes(ctx, type2ID, 2, 0); err != nil {
		return err
	}
	if err := s.waitActionExpired(ctx, type2ID); err != nil {
		return err
	}
	if err := s.execute(ctx, type2ID); err != nil {
		return err
	}
	action, err = s.waitActionRound(ctx, type2ID, 1, 0, 0)
	if err != nil {
		return err
	}
	liquidityMid, err := s.rpc.getLiquidityInfo(ctx)
	if err != nil {
		return err
	}
	if action.Executed || liquidityMid.IsHalted != liquidityBefore.IsHalted {
		return errors.New("Type2 action executed at the exact 2 of 4 yes-vote boundary")
	}
	fmt.Println("PASS Type2 round 0 ratchets instead of executing with 2 of 4 Yes votes")
	if action.ActivePillarThreshold != 40 || action.DirectionalThreshold != 55 {
		return fmt.Errorf("round 1 Type2 schedule = active %d, directional %d", action.ActivePillarThreshold, action.DirectionalThreshold)
	}
	fmt.Println("PASS Type2 action ratcheted to round 1 with 40% active / 55% directional thresholds")

	for _, pillar := range devnetPillars[:2] {
		if err := s.voteCurrentRound(ctx, pillar, type2ID, definition.VoteYes); err != nil {
			return err
		}
	}
	if err := s.waitActionVotes(ctx, type2ID, 2, 0); err != nil {
		return err
	}
	if err := s.execute(ctx, type2ID); err != nil {
		return err
	}
	if err := s.waitNoUnreceived(ctx, types.LiquidityContract); err != nil {
		return err
	}
	if err := s.waitActionExecuted(ctx, type2ID); err != nil {
		return err
	}
	if err := s.waitLiquidityHalted(ctx, targetHalted); err != nil {
		return err
	}
	liquidityAfter, err := s.rpc.getLiquidityInfo(ctx)
	if err != nil {
		return err
	}
	if liquidityAfter.IsHalted != targetHalted {
		return fmt.Errorf("liquidity halt state = %t, want %t", liquidityAfter.IsHalted, targetHalted)
	}
	fmt.Println("PASS Type2 action executes with 2 of 4 Yes votes after ratcheting to round 1")
	return nil
}

func (s *scenario) run(ctx context.Context, label string, sporkOnly bool) error {
	if err := s.waitFrontierHeight(ctx, 2); err != nil {
		return err
	}
	fmt.Println("PASS devnet frontier is beyond genesis so spork enforcement is active")

	if err := s.requireGovernanceActive(ctx); err != nil {
		return err
	}
	fmt.Println("PASS governance spork is active")

	if err := s.requirePillarCount(ctx, uint32(len(devnetPillars))); err != nil {
		return err
	}
	fmt.Printf("PASS devnet has %d active pillars for ratchet threshold testing\n", len(devnetPillars))

	badData := definition.ABICommon.PackMethodPanic(definition.DonateMethodName)
	badProposal := definition.ABIGovernance.PackMethodPanic(
		definition.ProposeActionMethodName,
		"bad-"+label,
		"unsupported destination should fail",
		"https://zenon.network",
		types.AcceleratorContract,
		base64.StdEncoding.EncodeToString(badData),
	)
	if err := s.expectRejected(ctx, s.key(operatorIndex), types.GovernanceContract, types.ZnnTokenStandard, big.NewInt(constants.Decimals), badProposal, "address cannot call this method"); err != nil {
		return err
	}
	fmt.Println("PASS unsupported governance destination is rejected")

	if err := s.runType1Ratchet(ctx, label); err != nil {
		return err
	}
	if err := s.runType1NoVoteExpiry(ctx, label); err != nil {
		return err
	}
	if sporkOnly {
		return nil
	}
	return s.runType2Ratchet(ctx, label)
}

func waitUntil(ctx context.Context, timeout, interval time.Duration, fn func() (bool, error)) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		ok, err := fn()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out after %s", timeout)
		case <-ticker.C:
		}
	}
}

func fusedPlasmaFor(required *requiredPoWResult) uint64 {
	if required.AvailablePlasma > required.BasePlasma {
		return required.BasePlasma
	}
	return required.AvailablePlasma
}

func keyStoreFromEntropy(entropy []byte) (*wallet.KeyStore, error) {
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, err
	}
	ks := &wallet.KeyStore{
		Entropy:  entropy,
		Seed:     bip39.NewSeed(mnemonic, ""),
		Mnemonic: mnemonic,
	}
	_, kp, err := ks.DeriveForIndexPath(0)
	if err != nil {
		return nil, err
	}
	ks.BaseAddress = kp.Address
	return ks, nil
}

func deriveKeys() (map[uint32]*wallet.KeyPair, error) {
	entropy, err := bip39.EntropyFromMnemonic(devMnemonic)
	if err != nil {
		return nil, err
	}
	ks, err := keyStoreFromEntropy(entropy)
	if err != nil {
		return nil, err
	}

	indexes := []uint32{1, 5, 7, 11}
	keys := make(map[uint32]*wallet.KeyPair, len(indexes))
	for _, index := range indexes {
		_, kp, err := ks.DeriveForIndexPath(index)
		if err != nil {
			return nil, err
		}
		keys[index] = kp
	}
	return keys, nil
}

func defaultLabel() string {
	return fmt.Sprintf("gov%d", time.Now().Unix()%1000000)
}

func main() {
	rpcURL := flag.String("rpc", defaultRPCURL, "HTTP JSON-RPC endpoint")
	timeout := flag.Duration("timeout", 2*time.Minute, "wait timeout per scenario step")
	label := flag.String("label", defaultLabel(), "short unique label for action and spork names")
	sporkOnly := flag.Bool("spork-only", false, "only run the governance create-spork scenario")
	flag.Parse()

	if len(*label) > maxLabelLength {
		fmt.Fprintf(os.Stderr, "label must be %d characters or fewer\n", maxLabelLength)
		os.Exit(2)
	}

	keys, err := deriveKeys()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx := context.Background()
	s := &scenario{
		rpc:     newRPCClient(*rpcURL),
		keys:    keys,
		timeout: *timeout,
	}
	if err := s.run(ctx, *label, *sporkOnly); err != nil {
		fmt.Fprintln(os.Stderr, "FAILED:", err)
		os.Exit(1)
	}
	fmt.Println("PASS governance devnet scenarios completed")
}
