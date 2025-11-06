package subscribe

import (
	"testing"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
)

// TestNewAccountBlockFiltered_NoDuplicates tests that descendant blocks
// already present as top-level blocks in the momentum are not duplicated
// when flattening the parent block's descendants.
//
// This test verifies the fix for duplicate unreceived block notifications
// that occurred when embedded contracts (like plasma.CancelFuse) returned
// refund transactions as descendant blocks.
func TestNewAccountBlockFiltered_NoDuplicates(t *testing.T) {
	// Simulate a plasma CancelFuse scenario:
	// 1. Descendant block (refund send) at height 4
	// 2. Parent block (contract receive) at height 5 with descendant reference

	descendantHash := types.HexToHashPanic("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	parentHash := types.HexToHashPanic("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	userAddress := types.ParseAddressPanic("z1qqdt06lnwz57x38rwlyutcx5wgrtl0ynkfe3kv")

	// Create the descendant block (refund send from contract to user)
	descendantBlock := &nom.AccountBlock{
		BlockType:     nom.BlockTypeContractSend,
		Hash:          descendantHash,
		Height:        4,
		Address:       types.PlasmaContract,
		ToAddress:     userAddress,
		FromBlockHash: types.ZeroHash,
	}

	// Create the parent block (contract receive) with descendant reference
	parentBlock := &nom.AccountBlock{
		BlockType:        nom.BlockTypeContractReceive,
		Hash:             parentHash,
		Height:           5,
		Address:          types.PlasmaContract,
		ToAddress:        types.ZeroAddress,
		FromBlockHash:    types.HexToHashPanic("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
		DescendantBlocks: []*nom.AccountBlock{descendantBlock},
	}

	// Build the top-level blocks set (both blocks are in the momentum)
	topLevelBlocks := make(map[types.Hash]bool)
	topLevelBlocks[descendantHash] = true
	topLevelBlocks[parentHash] = true

	// Test descendant block processing - should return only itself, no recursion
	descendantResult := newAccountBlockFiltered(descendantBlock, topLevelBlocks)
	if len(descendantResult) != 1 {
		t.Errorf("Expected 1 block from descendant, got %d", len(descendantResult))
	}
	if descendantResult[0].Hash != descendantHash {
		t.Errorf("Expected descendant hash %v, got %v", descendantHash, descendantResult[0].Hash)
	}

	// Test parent block processing - should return only itself, skip descendant (already top-level)
	parentResult := newAccountBlockFiltered(parentBlock, topLevelBlocks)
	if len(parentResult) != 1 {
		t.Errorf("Expected 1 block from parent (descendant should be skipped), got %d", len(parentResult))
	}
	if parentResult[0].Hash != parentHash {
		t.Errorf("Expected parent hash %v, got %v", parentHash, parentResult[0].Hash)
	}

	// Verify total unique blocks across both calls
	uniqueHashes := make(map[types.Hash]bool)
	for _, block := range descendantResult {
		uniqueHashes[block.Hash] = true
	}
	for _, block := range parentResult {
		uniqueHashes[block.Hash] = true
	}
	if len(uniqueHashes) != 2 {
		t.Errorf("Expected 2 unique blocks total, got %d", len(uniqueHashes))
	}
}

// TestNewAccountBlockFiltered_NestedDescendants tests that truly nested
// descendants (not in top-level) are still flattened correctly
func TestNewAccountBlockFiltered_NestedDescendants(t *testing.T) {
	// Simulate a hypothetical scenario with a nested descendant that's
	// NOT in the momentum as a top-level block

	nestedHash := types.HexToHashPanic("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	parentHash := types.HexToHashPanic("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	userAddress := types.ParseAddressPanic("z1qqdt06lnwz57x38rwlyutcx5wgrtl0ynkfe3kv")

	// Create a nested descendant (not in top-level momentum)
	nestedDescendant := &nom.AccountBlock{
		BlockType:     nom.BlockTypeContractSend,
		Hash:          nestedHash,
		Height:        4,
		Address:       types.PlasmaContract,
		ToAddress:     userAddress,
		FromBlockHash: types.ZeroHash,
	}

	// Create parent block with nested descendant
	parentBlock := &nom.AccountBlock{
		BlockType:        nom.BlockTypeContractReceive,
		Hash:             parentHash,
		Height:           5,
		Address:          types.PlasmaContract,
		ToAddress:        types.ZeroAddress,
		FromBlockHash:    types.HexToHashPanic("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
		DescendantBlocks: []*nom.AccountBlock{nestedDescendant},
	}

	// Top-level blocks only includes parent, NOT the nested descendant
	topLevelBlocks := make(map[types.Hash]bool)
	topLevelBlocks[parentHash] = true

	// Test parent block - should flatten and return both parent AND nested descendant
	result := newAccountBlockFiltered(parentBlock, topLevelBlocks)
	if len(result) != 2 {
		t.Errorf("Expected 2 blocks (parent + nested descendant), got %d", len(result))
	}

	// Verify we got both blocks
	foundParent := false
	foundNested := false
	for _, block := range result {
		if block.Hash == parentHash {
			foundParent = true
		}
		if block.Hash == nestedHash {
			foundNested = true
		}
	}

	if !foundParent {
		t.Error("Parent block not found in result")
	}
	if !foundNested {
		t.Error("Nested descendant not found in result")
	}
}

// TestNewAccountBlockFiltered_EmptyDescendants tests that blocks with
// no descendants work correctly
func TestNewAccountBlockFiltered_EmptyDescendants(t *testing.T) {
	blockHash := types.HexToHashPanic("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	userAddress1 := types.ParseAddressPanic("z1qqdt06lnwz57x38rwlyutcx5wgrtl0ynkfe3kv")
	userAddress2 := types.ParseAddressPanic("z1qzal6c5s9rjnnxd2z7dvdhjxpmmj4fmw56a0mz")

	block := &nom.AccountBlock{
		BlockType:        nom.BlockTypeUserSend,
		Hash:             blockHash,
		Height:           10,
		Address:          userAddress1,
		ToAddress:        userAddress2,
		DescendantBlocks: nil, // No descendants
	}

	topLevelBlocks := make(map[types.Hash]bool)
	topLevelBlocks[blockHash] = true

	result := newAccountBlockFiltered(block, topLevelBlocks)
	if len(result) != 1 {
		t.Errorf("Expected 1 block, got %d", len(result))
	}
	if result[0].Hash != blockHash {
		t.Errorf("Expected block hash %v, got %v", blockHash, result[0].Hash)
	}
}
