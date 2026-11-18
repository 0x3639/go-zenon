//go:build e2e

package e2e

import (
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
)

// The TestFast groups need the test build with fast constants
// (docs/TESTING.md, procedure P11): a 10-minute epoch, locks of minutes and
// a bridge administrator whose key the driver holds. They cover what cannot
// be reached on a stock binary in a test run: expiry of locks, rewards and
// administration. The rules are the node's own; only the durations and one
// address differ.

// frontierTime is the timestamp of the frontier momentum.
func frontierTime(t *testing.T) int64 {
	t.Helper()
	m, err := net.Frontier()
	if err != nil {
		t.Fatal(err)
	}
	return int64(m.TimestampUnix)
}

func listOf(v obj) []interface{} {
	list, _ := v["list"].([]interface{})
	return list
}

func TestFastPlasma(t *testing.T) {
	fastGroup(t)
	a := newAccount(t, 110, 0, 100, 1000)
	plasma := types.PlasmaContract
	var id types.Hash

	t.Run("FP-1_cancel_after_expiry_returns_the_qsr", func(t *testing.T) {
		cover(t, "E1", "contract-method", "plasma.CancelFuse")
		cover(t, "E12", "contract-method", "plasma.CancelFuse")
		before := bal(t, a.Address, qsr)
		id = callOK(t, a, plasma, definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, a.Address), qsr, coins(50)).Send.Hash
		cancel := definition.ABIPlasma.PackMethodPanic(definition.CancelFuseMethodName, id)
		callFail(t, a, plasma, cancel, znn, units(0)) // still inside the lock
		eventually(t, 6*time.Minute, "the fusion becomes revocable", func() bool {
			var entries obj
			get(t, &entries, "embedded.plasma.getEntriesByAddress", a.Address, 0, 10)
			list := listOf(entries)
			return len(list) == 1 && str(list[0].(obj)["isRevocable"]) == "true"
		})
		callOK(t, a, plasma, cancel, znn, units(0))
		receiveAll(t, a, 1)
		wantBig(t, "QSR after the cancel", bal(t, a.Address, qsr), before)
		var entries obj
		get(t, &entries, "embedded.plasma.getEntriesByAddress", a.Address, 0, 10)
		if str(entries["count"]) != "0" {
			t.Errorf("fusion entries after the cancel: %v", entries)
		}
	})

	t.Run("FP-2_second_cancel_fails", func(t *testing.T) {
		cover(t, "E4", "contract-method", "plasma.CancelFuse")
		callFail(t, a, plasma, definition.ABIPlasma.PackMethodPanic(definition.CancelFuseMethodName, id), znn, units(0))
	})
}

func TestFastStake(t *testing.T) {
	fastGroup(t)
	a := newAccount(t, 112, 200, 0, 1500)
	idle := newAccount(t, 113, 0, 0, 500)
	stake := types.StakeContract
	collect := definition.ABIStake.PackMethodPanic(definition.CollectRewardMethodName)

	t.Run("FS-1_cancel_after_expiry_returns_the_znn", func(t *testing.T) {
		cover(t, "E1", "contract-method", "stake.Cancel")
		cover(t, "E12", "contract-method", "stake.Cancel")
		before := bal(t, a.Address, znn)
		id := callOK(t, a, stake, definition.ABIStake.PackMethodPanic(definition.StakeMethodName, constants.StakeTimeMinSec), znn, coins(10)).Send.Hash
		// A second, long stake keeps earning for FS-2.
		callOK(t, a, stake, definition.ABIStake.PackMethodPanic(definition.StakeMethodName, constants.StakeTimeMaxSec), znn, coins(100))
		var entries obj
		get(t, &entries, "embedded.stake.getEntriesByAddress", a.Address, 0, 10)
		var expiry int64
		for _, e := range listOf(entries) {
			if str(e.(obj)["id"]) == id.String() {
				fmt.Sscan(str(e.(obj)["expirationTimestamp"]), &expiry)
			}
		}
		if expiry == 0 {
			t.Fatalf("stake entry not found: %v", entries)
		}
		cancel := definition.ABIStake.PackMethodPanic(definition.CancelStakeMethodName, id)
		eventually(t, 5*time.Minute, "the stake expires", func() bool { return frontierTime(t) >= expiry })
		callOK(t, a, stake, cancel, znn, units(0))
		receiveAll(t, a, 1)
		wantBig(t, "ZNN after the cancel", bal(t, a.Address, znn), new(big.Int).Sub(before, coins(100)))
		// A cancelled entry stays, with amount zero, until the next reward
		// update. A second cancel is therefore accepted, and pays nothing:
		// the contract sends a zero-amount block.
		after := bal(t, a.Address, znn)
		res := callOK(t, a, stake, cancel, znn, units(0))
		if len(res.Receive.DescendantBlocks) != 1 || res.Receive.DescendantBlocks[0].Amount.Sign() != 0 {
			t.Fatalf("second cancel of the same stake: descendants %v, want one block of amount 0", res.Receive.DescendantBlocks)
		}
		receiveAll(t, a, 1)
		wantBig(t, "ZNN after a second cancel", bal(t, a.Address, znn), after)
		cover(t, "E4", "contract-method", "stake.Cancel")
	})

	t.Run("FS-2_collect_the_reward_of_a_finished_epoch", func(t *testing.T) {
		cover(t, "E1", "contract-method", "stake.CollectReward", "common.CollectReward")
		var reward obj
		eventually(t, 25*time.Minute, "a stake reward is credited after an epoch", func() bool {
			get(t, &reward, "embedded.stake.getUncollectedReward", a.Address)
			return bigOf(t, reward["qsrAmount"]).Sign() > 0
		})
		amount := bigOf(t, reward["qsrAmount"])
		before := bal(t, a.Address, qsr)
		callOK(t, a, stake, collect, znn, units(0))
		receiveAll(t, a, 1)
		wantBig(t, "QSR after collecting", bal(t, a.Address, qsr), new(big.Int).Add(before, amount))
		get(t, &reward, "embedded.stake.getUncollectedReward", a.Address)
		if bigOf(t, reward["qsrAmount"]).Sign() != 0 {
			t.Errorf("uncollected reward after collecting: %v", reward)
		}
		var page obj
		get(t, &page, "embedded.stake.getFrontierRewardByPage", a.Address, 0, 10)
		if str(page["count"]) == "0" {
			t.Errorf("reward history is empty after a reward: %v", page)
		}
		t.Logf("stake reward for the first epoch: %s QSR units", amount)
	})

	t.Run("FS-3_collect_with_nothing_to_collect_fails", func(t *testing.T) {
		cover(t, "E4", "contract-method", "stake.CollectReward")
		callFail(t, idle, stake, collect, znn, units(0))
	})
}

func TestFastPillar(t *testing.T) {
	fastGroup(t)
	delegator := newAccount(t, 114, 1000, 0, 1000)
	pillar := types.PillarContract
	pack := func(name string, args ...interface{}) []byte {
		return definition.ABIPillars.PackMethodPanic(name, args...)
	}
	collect := pack(definition.CollectRewardMethodName)

	// Delegate first: the reward of an epoch goes to those delegating in it.
	callOK(t, delegator, pillar, pack(definition.DelegateMethodName, "dev1"), znn, units(0))

	var cost string
	if err := net.Call(&cost, "embedded.pillar.getQsrRegistrationCost"); err != nil {
		t.Fatal(err)
	}
	costQsr, _ := new(big.Int).SetString(cost, 10)
	costCoins := new(big.Int).Div(costQsr, coins(1)).Int64()
	owner := fundFrom(t, net.Key(8), 115, 15010, costCoins+10)
	name := "e2e-fast-" + runTag
	producer := net.Key(runBase + 116).Address

	t.Run("FPI-1_register_with_the_qsr_deposit", func(t *testing.T) {
		cover(t, "E1", "contract-method", "pillars.Register", "pillars.DepositQsr")
		callOK(t, owner, pillar, pack(definition.DepositQsrMethodName), qsr, costQsr)
		before := bal(t, owner.Address, znn)
		callOK(t, owner, pillar, pack(definition.RegisterMethodName, name, producer, owner.Address, uint8(0), uint8(100)), znn, constants.PillarStakeAmount)
		wantBig(t, "ZNN staked", bal(t, owner.Address, znn), new(big.Int).Sub(before, constants.PillarStakeAmount))
		var info, all obj
		get(t, &info, "embedded.pillar.getByName", name)
		get(t, &all, "embedded.pillar.getAll", 0, 10)
		if str(info["ownerAddress"]) != owner.Address.String() || str(info["producerAddress"]) != producer.String() || str(all["count"]) != "4" {
			t.Errorf("pillar read back: %v, count %v", info, all["count"])
		}
		var deposited string
		get(t, &deposited, "embedded.pillar.getDepositedQsr", owner.Address)
		if deposited != "0" {
			t.Errorf("deposited QSR after registration: %s, want 0 (it is burned)", deposited)
		}
		var free bool
		get(t, &free, "embedded.pillar.checkNameAvailability", name)
		if free {
			t.Errorf("name of a registered pillar reported as available")
		}
	})

	t.Run("FPI-3_update_by_the_owner", func(t *testing.T) {
		cover(t, "E1", "contract-method", "pillars.UpdatePillar")
		callOK(t, owner, pillar, pack(definition.UpdatePillarMethodName, name, producer, owner.Address, uint8(10), uint8(90)), znn, units(0))
		var info obj
		get(t, &info, "embedded.pillar.getByName", name)
		t.Logf("after the update, with EpochRewardTerms enforced: %v", info)
	})

	t.Run("FPI-4_revoke_inside_the_lock_fails_then_succeeds_in_the_window", func(t *testing.T) {
		cover(t, "E1", "contract-method", "pillars.Revoke")
		cover(t, "E12", "contract-method", "pillars.Revoke")
		revoke := pack(definition.RevokeMethodName, name)
		var info obj
		get(t, &info, "embedded.pillar.getByName", name)
		if str(info["isRevocable"]) == "false" {
			callFail(t, owner, pillar, revoke, znn, units(0))
		}
		eventually(t, 6*time.Minute, "the pillar enters its revoke window", func() bool {
			get(t, &info, "embedded.pillar.getByName", name)
			return str(info["isRevocable"]) == "true"
		})
		before := bal(t, owner.Address, znn)
		callOK(t, owner, pillar, revoke, znn, units(0))
		receiveAll(t, owner, 1)
		wantBig(t, "ZNN after the revoke", bal(t, owner.Address, znn), new(big.Int).Add(before, constants.PillarStakeAmount))
		var all obj
		get(t, &all, "embedded.pillar.getAll", 0, 10)
		active := 0
		for _, p := range listOf(all) {
			if str(p.(obj)["revokeTimestamp"]) == "0" {
				active++
			}
		}
		if active != 3 {
			t.Errorf("active pillars after the revoke: %d, want 3 (%v)", active, all)
		}
		callFail(t, owner, pillar, revoke, znn, units(0)) // already revoked
	})

	t.Run("FPI-5_delegator_collects_the_reward_of_a_finished_epoch", func(t *testing.T) {
		cover(t, "E1", "contract-method", "pillars.CollectReward")
		var reward obj
		eventually(t, 25*time.Minute, "a delegation reward is credited after an epoch", func() bool {
			get(t, &reward, "embedded.pillar.getUncollectedReward", delegator.Address)
			return bigOf(t, reward["znnAmount"]).Sign() > 0
		})
		amount := bigOf(t, reward["znnAmount"])
		before := bal(t, delegator.Address, znn)
		callOK(t, delegator, pillar, collect, znn, units(0))
		receiveAll(t, delegator, 1)
		wantBig(t, "ZNN after collecting", bal(t, delegator.Address, znn), new(big.Int).Add(before, amount))
		var page obj
		get(t, &page, "embedded.pillar.getFrontierRewardByPage", delegator.Address, 0, 10)
		t.Logf("delegation reward for the first epoch: %s ZNN units", amount)
	})

	t.Run("FPI-6_epoch_history_is_recorded", func(t *testing.T) {
		var history, byEpoch obj
		get(t, &history, "embedded.pillar.getPillarEpochHistory", "dev1", 0, 10)
		get(t, &byEpoch, "embedded.pillar.getPillarsHistoryByEpoch", 0, 0, 10)
		if str(history["count"]) == "0" || str(byEpoch["count"]) == "0" {
			t.Errorf("no epoch history after a finished epoch: %v / %v", history, byEpoch)
		}
	})
}

func TestFastSentinel(t *testing.T) {
	fastGroup(t)
	a := fundFrom(t, net.Key(8), 118, 5010, 50100)
	sentinel := types.SentinelContract
	pack := func(name string) []byte { return definition.ABISentinel.PackMethodPanic(name) }

	t.Run("FSE-1_deposit_then_withdraw_qsr", func(t *testing.T) {
		cover(t, "E1", "contract-method", "sentinel.WithdrawQsr", "sentinel.DepositQsr")
		before := bal(t, a.Address, qsr)
		callOK(t, a, sentinel, pack(definition.DepositQsrMethodName), qsr, coins(10))
		callOK(t, a, sentinel, pack(definition.WithdrawQsrMethodName), znn, units(0))
		receiveAll(t, a, 1)
		wantBig(t, "QSR after the withdraw", bal(t, a.Address, qsr), before)
	})

	t.Run("FSE-2_revoke_in_the_window_returns_the_stake", func(t *testing.T) {
		cover(t, "E1", "contract-method", "sentinel.Revoke")
		cover(t, "E12", "contract-method", "sentinel.Revoke")
		znnBefore, qsrBefore := bal(t, a.Address, znn), bal(t, a.Address, qsr)
		callOK(t, a, sentinel, pack(definition.DepositQsrMethodName), qsr, constants.SentinelQsrDepositAmount)
		callOK(t, a, sentinel, pack(definition.RegisterSentinelMethodName), znn, constants.SentinelZnnRegisterAmount)
		var info obj
		eventually(t, 6*time.Minute, "the sentinel enters its revoke window", func() bool {
			get(t, &info, "embedded.sentinel.getByOwner", a.Address)
			return str(info["isRevocable"]) == "true"
		})
		res := callOK(t, a, sentinel, pack(definition.RevokeSentinelMethodName), znn, units(0))
		receiveAll(t, a, len(res.Receive.DescendantBlocks))
		wantBig(t, "ZNN after the revoke", bal(t, a.Address, znn), znnBefore)
		wantBig(t, "QSR after the revoke", bal(t, a.Address, qsr), qsrBefore)
		get(t, &info, "embedded.sentinel.getByOwner", a.Address)
		if str(info["active"]) == "true" {
			t.Errorf("sentinel still active after the revoke: %v", info)
		}
		callFail(t, a, sentinel, pack(definition.RevokeSentinelMethodName), znn, units(0)) // already revoked
	})
}

func TestFastAccelerator(t *testing.T) {
	fastGroup(t)
	a := newAccount(t, 120, 10, 10, 2000)
	az := types.AcceleratorContract
	pack := func(name string, args ...interface{}) []byte {
		return definition.ABIAccelerator.PackMethodPanic(name, args...)
	}
	owners := map[string]uint32{"dev1": IdxOwner1, "dev2": IdxOwner2, "dev3": IdxOwner3}
	voteAll := func(t *testing.T, id types.Hash) {
		t.Helper()
		for name, idx := range owners {
			callOK(t, net.Key(idx), az, pack(definition.VoteByNameMethodName, id, name, definition.VoteYes), znn, units(0))
		}
	}
	// The contract acts on votes when it is updated. Pillars send the update
	// by themselves; the driver also sends one, and a refusal for being too
	// soon after the last update is expected.
	update := func() {
		if res, err := call(a, az, pack(definition.UpdateMethodName), znn, units(0)); err == nil && res.OK() {
			cov.add("contract-method", "accelerator.Update", "E1:FAZ-1")
			cov.add("contract-method", "common.Update", "E1:FAZ-1")
		}
	}
	var project types.Hash

	t.Run("FAZ-1_project_accepted_by_the_pillars", func(t *testing.T) {
		project = callOK(t, a, az, pack(definition.CreateProjectMethodName, "e2e fast "+runTag, "funded by the driver", "e2e.test", coins(10), coins(100)), znn, constants.ProjectCreationAmount).Send.Hash
		voteAll(t, project)
		var got obj
		eventually(t, 6*time.Minute, "the project is accepted", func() bool {
			update()
			get(t, &got, "embedded.accelerator.getProjectById", project)
			return str(got["status"]) == fmt.Sprint(definition.ActiveStatus)
		})
	})

	t.Run("FAZ-2_phase_added_updated_voted_and_paid", func(t *testing.T) {
		cover(t, "E1", "contract-method", "accelerator.AddPhase", "accelerator.UpdatePhase")
		znnBefore, qsrBefore := bal(t, a.Address, znn), bal(t, a.Address, qsr)
		phase := callOK(t, a, az, pack(definition.AddPhaseMethodName, project, "phase one", "first", "e2e.test", coins(5), coins(50)), znn, units(0)).Send.Hash
		// Changing a phase resets its votes; the new amounts are what gets paid.
		callOK(t, a, az, pack(definition.UpdatePhaseMethodName, project, "phase one", "first, revised", "e2e.test", coins(6), coins(60)), znn, units(0))
		var got obj
		get(t, &got, "embedded.accelerator.getProjectById", project)
		phases, _ := got["phases"].([]interface{})
		if len(phases) != 1 {
			t.Fatalf("phases of the project: %v", got["phases"])
		}
		current := types.HexToHashPanic(str(phases[0].(obj)["phase"].(obj)["id"]))
		t.Logf("phase id at creation %s, after the update %s", phase, current)
		voteAll(t, current)
		eventually(t, 6*time.Minute, "the phase is paid", func() bool {
			update()
			var p obj
			get(t, &p, "embedded.accelerator.getPhaseById", current)
			return str(p["phase"].(obj)["status"]) == fmt.Sprint(definition.PaidStatus)
		})
		receiveAll(t, a, 2)
		wantBig(t, "ZNN paid for the phase", bal(t, a.Address, znn), new(big.Int).Add(znnBefore, coins(6)))
		wantBig(t, "QSR paid for the phase", bal(t, a.Address, qsr), new(big.Int).Add(qsrBefore, coins(60)))
	})
}

func TestFastBridgeAdmin(t *testing.T) {
	fastGroup(t)
	admin := net.Key(9) // constants.InitialBridgeAdministrator on the test build
	stranger := newAccount(t, 122, 0, 0, 1500)
	callOK(t, net.Key(IdxFunder), types.PlasmaContract, definition.ABIPlasma.PackMethodPanic(definition.FuseMethodName, admin.Address), qsr, coins(3000))
	bridge := types.BridgeContract
	pack := func(name string, args ...interface{}) []byte {
		return definition.ABIBridge.PackMethodPanic(name, args...)
	}
	const class, chain = uint32(2), uint32(31337)
	contract := "0x" + strings.Repeat("ab", 20)
	tokenAddr := "0x" + strings.Repeat("cd", 20)
	setNetwork := func(class, chain uint32, name, contract, meta string) []byte {
		return pack(definition.SetNetworkMethodName, class, chain, name, contract, meta)
	}
	setPair := func(zts types.ZenonTokenStandard, owned bool, fee, delay uint32, meta string) []byte {
		return pack(definition.SetTokenPairMethod, class, chain, zts, tokenAddr, true, true, owned, units(1), fee, delay, meta)
	}

	t.Run("FB-1_administrator_is_the_drivers_account", func(t *testing.T) {
		var info obj
		get(t, &info, "embedded.bridge.getBridgeInfo")
		if str(info["administrator"]) != admin.Address.String() {
			t.Fatalf("bridge administrator is %v, want %s", info["administrator"], admin.Address)
		}
	})

	t.Run("FB-2_set_network", func(t *testing.T) {
		cover(t, "E1", "contract-method", "bridge.SetNetwork")
		cover(t, "E2", "contract-method", "bridge.SetNetwork")
		cover(t, "E3", "contract-method", "bridge.SetNetwork")
		cover(t, "E6", "contract-method", "bridge.SetNetwork")
		callFail(t, stranger, bridge, setNetwork(class, chain, "e2e-net", contract, "{}"), znn, units(0))
		callReject(t, constants.ErrInvalidNetworkName.Error(), admin, bridge, setNetwork(class, chain, "ab", contract, "{}"), znn, units(0))
		callReject(t, constants.ErrInvalidNetworkName.Error(), admin, bridge, setNetwork(class, chain, strings.Repeat("n", 33), contract, "{}"), znn, units(0))
		callReject(t, constants.ErrForbiddenParam.Error(), admin, bridge, setNetwork(0, chain, "e2e-net", contract, "{}"), znn, units(0))
		callReject(t, constants.ErrForbiddenParam.Error(), admin, bridge, setNetwork(class, 0, "e2e-net", contract, "{}"), znn, units(0))
		callReject(t, constants.ErrInvalidContractAddress.Error(), admin, bridge, setNetwork(class, chain, "e2e-net", "not an address", "{}"), znn, units(0))
		callReject(t, constants.ErrInvalidJsonContent.Error(), admin, bridge, setNetwork(class, chain, "e2e-net", contract, "not json"), znn, units(0))
		callOK(t, admin, bridge, setNetwork(class, chain, "e2e-net", contract, "{}"), znn, units(0))
		var got, all obj
		get(t, &got, "embedded.bridge.getNetworkInfo", class, chain)
		get(t, &all, "embedded.bridge.getAllNetworks", 0, 10)
		if str(got["name"]) != "e2e-net" || str(all["count"]) != "1" {
			t.Errorf("network read back: %v, count %v", got, all["count"])
		}
	})

	t.Run("FB-3_set_token_pair", func(t *testing.T) {
		cover(t, "E1", "contract-method", "bridge.SetTokenPair")
		cover(t, "E2", "contract-method", "bridge.SetTokenPair")
		cover(t, "E3", "contract-method", "bridge.SetTokenPair")
		forbidden := constants.ErrForbiddenParam.Error()
		callFail(t, stranger, bridge, setPair(znn, false, 100, 2, "{}"), znn, units(0))
		callReject(t, forbidden, admin, bridge, setPair(znn, true, 100, 2, "{}"), znn, units(0)) // ZNN cannot be owned by the bridge
		callReject(t, forbidden, admin, bridge, setPair(znn, false, constants.MaximumFee+1, 2, "{}"), znn, units(0))
		callReject(t, forbidden, admin, bridge, setPair(znn, false, 100, 0, "{}"), znn, units(0))
		callReject(t, forbidden, admin, bridge, setPair(types.ZeroTokenStandard, false, 100, 2, "{}"), znn, units(0))
		callReject(t, constants.ErrInvalidJsonContent.Error(), admin, bridge, setPair(znn, false, 100, 2, "nope"), znn, units(0))
		// Setting a token pair is time-challenged: the first call opens the
		// challenge, and the same call applies it once the soft delay is over.
		cover(t, "E12", "contract-method", "bridge.SetTokenPair")
		good := setPair(znn, false, constants.MaximumFee, 2, "{}") // fee exactly at the maximum
		opened := callOK(t, admin, bridge, good, znn, units(0))
		var got, security, challenges obj
		get(t, &got, "embedded.bridge.getNetworkInfo", class, chain)
		if pairs, _ := got["tokenPairs"].([]interface{}); len(pairs) != 0 {
			// Only possible when an earlier run on this chain opened the same
			// challenge and its delay has passed since: this call applied it.
			t.Skipf("the challenge for these values was already open on this chain; run on a fresh chain to check the delay (%v)", pairs)
		}
		get(t, &challenges, "embedded.bridge.getTimeChallengesInfo")
		if str(challenges["count"]) == "0" {
			t.Errorf("no open time challenge after the first call: %v", challenges)
		}
		callFail(t, admin, bridge, good, znn, units(0)) // the delay is not over
		get(t, &security, "embedded.bridge.getSecurityInfo")
		var delay uint64
		fmt.Sscan(str(security["softDelay"]), &delay)
		due := opened.Receive.ConfirmationDetail.MomentumHeight + delay + 2
		if err := net.WaitHeight(due, 10*time.Minute); err != nil {
			t.Fatal(err)
		}
		callOK(t, admin, bridge, good, znn, units(0))
		get(t, &got, "embedded.bridge.getNetworkInfo", class, chain)
		pairs, _ := got["tokenPairs"].([]interface{})
		if len(pairs) != 1 || str(pairs[0].(obj)["tokenStandard"]) != znn.String() {
			t.Errorf("token pairs read back: %v", got["tokenPairs"])
		}
	})

	t.Run("FB-4_metadata", func(t *testing.T) {
		cover(t, "E1", "contract-method", "bridge.SetNetworkMetadata", "bridge.SetBridgeMetadata")
		cover(t, "E3", "contract-method", "bridge.SetNetworkMetadata", "bridge.SetBridgeMetadata")
		callFail(t, stranger, bridge, pack(definition.SetNetworkMetadataMethodName, class, chain, `{"a":1}`), znn, units(0))
		callFail(t, stranger, bridge, pack(definition.SetBridgeMetadataMethodName, `{"b":2}`), znn, units(0))
		callOK(t, admin, bridge, pack(definition.SetNetworkMetadataMethodName, class, chain, `{"a":1}`), znn, units(0))
		callOK(t, admin, bridge, pack(definition.SetBridgeMetadataMethodName, `{"b":2}`), znn, units(0))
		var netInfo, info obj
		get(t, &netInfo, "embedded.bridge.getNetworkInfo", class, chain)
		get(t, &info, "embedded.bridge.getBridgeInfo")
		if str(netInfo["metadata"]) != `{"a":1}` || str(info["metadata"]) != `{"b":2}` {
			t.Errorf("metadata read back: network %v, bridge %v", netInfo["metadata"], info["metadata"])
		}
	})

	t.Run("FB-5_orchestrator_info", func(t *testing.T) {
		cover(t, "E1", "contract-method", "bridge.SetOrchestratorInfo")
		cover(t, "E2", "contract-method", "bridge.SetOrchestratorInfo")
		cover(t, "E3", "contract-method", "bridge.SetOrchestratorInfo")
		orch := func(window uint64, threshold, confirmations, momentumTime uint32) []byte {
			return pack(definition.SetOrchestratorInfoMethodName, window, threshold, confirmations, momentumTime)
		}
		callReject(t, constants.ErrForbiddenParam.Error(), admin, bridge, orch(0, 1, 1, 10), znn, units(0))
		callReject(t, constants.ErrForbiddenParam.Error(), admin, bridge, orch(10, 0, 1, 10), znn, units(0))
		callFail(t, stranger, bridge, orch(10, 1, 1, 10), znn, units(0))
		callOK(t, admin, bridge, orch(10, 1, 1, 10), znn, units(0))
		var got obj
		get(t, &got, "embedded.bridge.getOrchestratorInfo")
		if str(got["windowSize"]) != "10" || str(got["confirmationsToFinality"]) != "1" {
			t.Errorf("orchestrator info read back: %v", got)
		}
	})

	t.Run("FB-6_remove_token_pair_and_network", func(t *testing.T) {
		cover(t, "E1", "contract-method", "bridge.RemoveTokenPair", "bridge.RemoveNetwork")
		cover(t, "E3", "contract-method", "bridge.RemoveTokenPair", "bridge.RemoveNetwork")
		removePair := pack(definition.RemoveTokenPairMethodName, class, chain, znn, tokenAddr)
		removeNet := pack(definition.RemoveNetworkMethodName, class, chain)
		callFail(t, stranger, bridge, removePair, znn, units(0))
		callFail(t, stranger, bridge, removeNet, znn, units(0))
		callOK(t, admin, bridge, removePair, znn, units(0))
		var got, all obj
		get(t, &got, "embedded.bridge.getNetworkInfo", class, chain)
		if pairs, _ := got["tokenPairs"].([]interface{}); len(pairs) != 0 {
			t.Errorf("token pairs after removal: %v", got["tokenPairs"])
		}
		callOK(t, admin, bridge, removeNet, znn, units(0))
		get(t, &all, "embedded.bridge.getAllNetworks", 0, 10)
		if str(all["count"]) != "0" {
			t.Errorf("networks after removal: %v", all)
		}
	})

	t.Run("FB-7_halt_and_unhalt", func(t *testing.T) {
		cover(t, "E1", "contract-method", "bridge.Halt", "bridge.Unhalt")
		cover(t, "E3", "contract-method", "bridge.Halt", "bridge.Unhalt")
		cover(t, "E4", "contract-method", "bridge.Halt", "bridge.Unhalt")
		halt, unhalt := pack(definition.HaltMethodName, ""), pack(definition.UnhaltMethodName)
		halted := func() string {
			var info obj
			get(t, &info, "embedded.bridge.getBridgeInfo")
			return str(info["halted"])
		}
		callFail(t, admin, bridge, unhalt, znn, units(0))  // not halted
		callFail(t, stranger, bridge, halt, znn, units(0)) // a stranger needs the TSS signature
		callOK(t, admin, bridge, halt, znn, units(0))
		if halted() != "true" {
			t.Fatalf("bridge not halted after the administrator's halt")
		}
		callFail(t, admin, bridge, halt, znn, units(0)) // already halted
		callFail(t, stranger, bridge, unhalt, znn, units(0))
		callOK(t, admin, bridge, unhalt, znn, units(0))
		if halted() != "false" {
			t.Errorf("bridge still halted after the administrator's unhalt")
		}
	})
}
