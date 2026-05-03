package implementation

import (
	"bytes"
	"encoding/hex"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/crypto"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
	"github.com/zenon-network/go-zenon/vm/vm_context"
)

var (
	htlcLog = common.EmbeddedLogger.New("contract", "htlc")
)

// GetHtlcProxyUnlockStatus loads the HtlcProxyUnlockStatus record from storage.
// This means there are really 3 states: Default, ExplicitDeny, ExplicitAllow
// And once an address has explicitly denied/allowed proxy unlock, it can no longer go back to using the default
// This is to ensure that if we ever change the default to deny, addresses that have called the Allow method will still work as expected
func GetHtlcProxyUnlockStatus(context vm_context.AccountVmContext, address types.Address) (bool, error) {
	info, err := definition.GetHtlcProxyUnlockInfo(context.Storage(), address)
	if err != nil {
		if err == constants.ErrDataNonExistent {
			// This defines the default behavior to allow proxy unlocks
			return true, nil
		}
		return false, err
	} else {
		return info.Allowed, nil
	}
}

func checkHtlc(param definition.CreateHtlcParam) error {

	if param.HashType != definition.HashTypeSHA3 && param.HashType != definition.HashTypeSHA256 {
		return constants.ErrInvalidHashType
	}

	if len(param.HashLock) != int(definition.HashTypeDigestSizes[param.HashType]) {
		return constants.ErrInvalidHashDigest
	}

	return nil
}

// CreateHtlcMethod creates a new hashed-timelock entry: locks
// the caller's transferred amount behind (hashLock, expirationTime)
// for the named hashLocked recipient. The recipient may unlock by
// revealing the preimage before expiration; afterwards the
// caller may [ReclaimHtlcMethod] the funds.
type CreateHtlcMethod struct {
	MethodName string
}

// GetPlasma returns this method's plasma requirement from the
// supplied table or method-specific configuration.
func (p *CreateHtlcMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedSimple, nil
}

// ValidateSendBlock decodes call data and checks token, amount,
// and method-specific send-block invariants.
func (p *CreateHtlcMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error

	param := new(definition.CreateHtlcParam)

	if err := definition.ABIHtlc.UnpackMethod(param, p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if err = checkHtlc(*param); err != nil {
		return err
	}

	// can't create empty htlcs
	if block.Amount.Sign() == 0 {
		htlcLog.Debug("invalid create - cannot create zero amount", "address", block.Address)
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIHtlc.PackMethod(p.MethodName,
		param.HashLocked,
		param.ExpirationTime,
		param.HashType,
		param.KeyMaxSize,
		param.HashLock,
	)
	return err
}

// ReceiveBlock applies the validated call to context and returns
// any descendant account blocks emitted by the method.
func (p *CreateHtlcMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		htlcLog.Debug("invalid create - syntactic validation failed", "address", sendBlock.Address, "reason", err)
		return nil, err
	}

	param := new(definition.CreateHtlcParam)
	err := definition.ABIHtlc.UnpackMethod(param, p.MethodName, sendBlock.Data)
	common.DealWithErr(err)

	momentum, err := context.GetFrontierMomentum()
	common.DealWithErr(err)

	// can't create htlc that is already expired
	if momentum.Timestamp.Unix() >= param.ExpirationTime {
		htlcLog.Debug("invalid create - cannot create already expired", "address", sendBlock.Address, "time", momentum.Timestamp.Unix(), "expiration-time", param.ExpirationTime)
		return nil, constants.ErrInvalidExpirationTime
	}

	htlcInfo := &definition.HtlcInfo{
		Id:             sendBlock.Hash,
		TimeLocked:     sendBlock.Address,
		HashLocked:     param.HashLocked,
		TokenStandard:  sendBlock.TokenStandard,
		Amount:         sendBlock.Amount,
		ExpirationTime: param.ExpirationTime,
		HashType:       param.HashType,
		KeyMaxSize:     param.KeyMaxSize,
		HashLock:       param.HashLock,
	}

	common.DealWithErr(htlcInfo.Save(context.Storage()))
	htlcLog.Debug("created", "htlcInfo", htlcInfo)
	return nil, nil
}

// ReclaimHtlcMethod refunds an HTLC entry to the original sender
// once expirationTime has elapsed without a successful unlock.
type ReclaimHtlcMethod struct {
	MethodName string
}

// GetPlasma returns this method's plasma requirement from the
// supplied table or method-specific configuration.
func (p *ReclaimHtlcMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedWWithdraw, nil
}

// ValidateSendBlock decodes call data and checks token, amount,
// and method-specific send-block invariants.
func (p *ReclaimHtlcMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error
	param := new(types.Hash)

	if err := definition.ABIHtlc.UnpackMethod(param, p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if block.Amount.Sign() > 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIHtlc.PackMethod(p.MethodName, param)
	return err
}

// ReceiveBlock applies the validated call to context and returns
// any descendant account blocks emitted by the method.
func (p *ReclaimHtlcMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		htlcLog.Debug("invalid reclaim - syntactic validation failed", "address", sendBlock.Address, "reason", err)
		return nil, err
	}

	id := new(types.Hash)
	err := definition.ABIHtlc.UnpackMethod(id, p.MethodName, sendBlock.Data)
	common.DealWithErr(err)

	htlcInfo, err := definition.GetHtlcInfo(context.Storage(), *id)
	if err == constants.ErrDataNonExistent {
		htlcLog.Debug("invalid reclaim - entry does not exist", "id", id, "address", sendBlock.Address)
		return nil, err
	}
	common.DealWithErr(err)

	// only timelocked can reclaim
	if htlcInfo.TimeLocked != sendBlock.Address {
		htlcLog.Debug("invalid reclaim - permission denied", "id", htlcInfo.Id, "address", sendBlock.Address)
		return nil, constants.ErrPermissionDenied
	}

	momentum, err := context.GetFrontierMomentum()
	common.DealWithErr(err)

	// can only reclaim after the entry is expired
	if momentum.Timestamp.Unix() < htlcInfo.ExpirationTime {
		htlcLog.Debug("invalid reclaim - entry not expired", "id", htlcInfo.Id, "address", sendBlock.Address, "time", momentum.Timestamp.Unix(), "expiration-time", htlcInfo.ExpirationTime)
		return nil, constants.ReclaimNotDue
	}

	common.DealWithErr(htlcInfo.Delete(context.Storage()))
	htlcLog.Debug("reclaimed", "htlcInfo", htlcInfo)

	return []*nom.AccountBlock{
		{
			Address:       types.HtlcContract,
			ToAddress:     htlcInfo.TimeLocked,
			BlockType:     nom.BlockTypeContractSend,
			Amount:        htlcInfo.Amount,
			TokenStandard: htlcInfo.TokenStandard,
			Data:          []byte{},
		},
	}, nil
}

// UnlockHtlcMethod claims an HTLC by revealing the preimage. The
// caller must be the hashLocked recipient (or have proxy-unlock
// permission via [AllowHtlcProxyUnlockMethod]) and the preimage
// must hash to the entry's hashLock under its declared hash type.
type UnlockHtlcMethod struct {
	MethodName string
}

// GetPlasma returns this method's plasma requirement from the
// supplied table or method-specific configuration.
func (p *UnlockHtlcMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedWWithdraw, nil
}

// ValidateSendBlock decodes call data and checks token, amount,
// and method-specific send-block invariants.
func (p *UnlockHtlcMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error
	param := new(definition.UnlockHtlcParam)

	if err := definition.ABIHtlc.UnpackMethod(param, p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if block.Amount.Sign() > 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIHtlc.PackMethod(p.MethodName, param.Id, param.Preimage)
	return err
}

// ReceiveBlock applies the validated call to context and returns
// any descendant account blocks emitted by the method.
func (p *UnlockHtlcMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		htlcLog.Debug("invalid unlock - syntactic validation failed", "address", sendBlock.Address, "reason", err)
		return nil, err
	}

	param := new(definition.UnlockHtlcParam)
	err := definition.ABIHtlc.UnpackMethod(param, p.MethodName, sendBlock.Data)
	common.DealWithErr(err)

	htlcInfo, err := definition.GetHtlcInfo(context.Storage(), param.Id)
	if err == constants.ErrDataNonExistent {
		htlcLog.Debug("invalid unlock - entry does not exist", "id", param.Id, "address", sendBlock.Address)
		return nil, err
	}
	common.DealWithErr(err)

	allowProxyUnlock, err := GetHtlcProxyUnlockStatus(context, htlcInfo.HashLocked)
	common.DealWithErr(err)
	// if proxy unlock is not allowed, only hashlocked can unlock
	if !allowProxyUnlock && sendBlock.Address != htlcInfo.HashLocked {
		htlcLog.Debug("invalid unlock - permission denied", "id", htlcInfo.Id, "address", sendBlock.Address)
		return nil, constants.ErrPermissionDenied
	}

	momentum, err := context.GetFrontierMomentum()
	common.DealWithErr(err)

	// can only unlock before expiration time
	if momentum.Timestamp.Unix() >= htlcInfo.ExpirationTime {
		htlcLog.Debug("invalid unlock - entry is expired", "id", htlcInfo.Id, "address", sendBlock.Address, "time", momentum.Timestamp.Unix(), "expiration-time", htlcInfo.ExpirationTime)
		return nil, constants.ErrExpired
	}

	if len(param.Preimage) > int(htlcInfo.KeyMaxSize) {
		htlcLog.Debug("invalid unlock - preimage size greater than entry KeyMaxSize", "id", htlcInfo.Id, "address", sendBlock.Address, "preimage-size", len(param.Preimage), "max-size", htlcInfo.KeyMaxSize)
		return nil, constants.ErrInvalidPreimage
	}

	var hashedPreimage []byte
	if htlcInfo.HashType == definition.HashTypeSHA3 {
		hashedPreimage = crypto.Hash(param.Preimage)
	} else if htlcInfo.HashType == definition.HashTypeSHA256 {
		hashedPreimage = crypto.HashSHA256(param.Preimage)
	} else {
		// shouldn't get here
	}

	if !bytes.Equal(hashedPreimage, htlcInfo.HashLock) {
		htlcLog.Debug("invalid unlock - wrong preimage", "id", htlcInfo.Id, "address", sendBlock.Address, "preimage", hex.EncodeToString(param.Preimage))
		return nil, constants.ErrInvalidPreimage
	}

	common.DealWithErr(htlcInfo.Delete(context.Storage()))
	htlcLog.Debug("unlocked", "htlcInfo", htlcInfo, "preimage", hex.EncodeToString(param.Preimage))

	return []*nom.AccountBlock{
		{
			Address:       types.HtlcContract,
			ToAddress:     htlcInfo.HashLocked,
			BlockType:     nom.BlockTypeContractSend,
			Amount:        htlcInfo.Amount,
			TokenStandard: htlcInfo.TokenStandard,
			Data:          []byte{},
		},
	}, nil
}

// DenyHtlcProxyUnlockMethod sets the per-account proxy-unlock
// flag to false (default), so only the hashLocked recipient may
// unlock entries.
type DenyHtlcProxyUnlockMethod struct {
	MethodName string
}

// GetPlasma returns this method's plasma requirement from the
// supplied table or method-specific configuration.
func (p *DenyHtlcProxyUnlockMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedSimple, nil
}

// ValidateSendBlock decodes call data and checks token, amount,
// and method-specific send-block invariants.
func (p *DenyHtlcProxyUnlockMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error

	if err := definition.ABIHtlc.UnpackEmptyMethod(p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if block.Amount.Sign() != 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIHtlc.PackMethod(p.MethodName)
	return err
}

// ReceiveBlock applies the validated call to context and returns
// any descendant account blocks emitted by the method.
func (p *DenyHtlcProxyUnlockMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		htlcLog.Debug("invalid create - syntactic validation failed", "address", sendBlock.Address, "reason", err)
		return nil, err
	}
	info := &definition.HtlcProxyUnlockInfo{
		Address: sendBlock.Address,
		Allowed: false,
	}
	if err := info.Save(context.Storage()); err != nil {
		return nil, err
	}
	htlcLog.Debug("deny proxy unlock", "address", sendBlock.Address)
	return nil, nil
}

// AllowHtlcProxyUnlockMethod sets the per-account proxy-unlock
// flag to true, so any caller in possession of the preimage may
// unlock entries on this account's behalf.
type AllowHtlcProxyUnlockMethod struct {
	MethodName string
}

// GetPlasma returns this method's plasma requirement from the
// supplied table or method-specific configuration.
func (p *AllowHtlcProxyUnlockMethod) GetPlasma(plasmaTable *constants.PlasmaTable) (uint64, error) {
	return plasmaTable.EmbeddedSimple, nil
}

// ValidateSendBlock decodes call data and checks token, amount,
// and method-specific send-block invariants.
func (p *AllowHtlcProxyUnlockMethod) ValidateSendBlock(block *nom.AccountBlock) error {
	var err error

	if err := definition.ABIHtlc.UnpackEmptyMethod(p.MethodName, block.Data); err != nil {
		return constants.ErrUnpackError
	}

	if block.Amount.Sign() != 0 {
		return constants.ErrInvalidTokenOrAmount
	}

	block.Data, err = definition.ABIHtlc.PackMethod(p.MethodName)
	return err
}

// ReceiveBlock applies the validated call to context and returns
// any descendant account blocks emitted by the method.
func (p *AllowHtlcProxyUnlockMethod) ReceiveBlock(context vm_context.AccountVmContext, sendBlock *nom.AccountBlock) ([]*nom.AccountBlock, error) {
	if err := p.ValidateSendBlock(sendBlock); err != nil {
		htlcLog.Debug("invalid create - syntactic validation failed", "address", sendBlock.Address, "reason", err)
		return nil, err
	}
	info := &definition.HtlcProxyUnlockInfo{
		Address: sendBlock.Address,
		Allowed: true,
	}
	if err := info.Save(context.Storage()); err != nil {
		return nil, err
	}
	htlcLog.Debug("allow proxy unlock", "address", sendBlock.Address)
	return nil, nil
}
