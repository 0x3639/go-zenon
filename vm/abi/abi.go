package abi

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/zenon-network/go-zenon/common"
)

// ABIContract is a parsed contract ABI: the methods callers can pack
// into [nom.AccountBlock.Data], plus the storage-record `variable`
// shapes the contracts use to (de)serialize on-chain state.
type ABIContract struct {
	Methods   map[string]Method
	Variables map[string]Variable
}

// JSONToABIContract parses a JSON-encoded ABI from reader. Panics
// through [common.DealWithErr] on any parse failure — ABIs are
// build-time constants, so a parse failure is a programmer error.
func JSONToABIContract(reader io.Reader) ABIContract {
	dec := json.NewDecoder(reader)

	var abi ABIContract
	if err := dec.Decode(&abi); err != nil {
		common.DealWithErr(err)
		return ABIContract{}
	}

	return abi
}

// PackMethod encodes a method call: prefixes the resulting bytes with
// the 4-byte method id and follows with the ABI-encoded arguments.
// Returns an error when name is unknown or the arguments do not
// match the method signature.
func (abi ABIContract) PackMethod(name string, args ...interface{}) ([]byte, error) {
	method, exist := abi.Methods[name]
	if !exist {
		return nil, errMethodNotFound(name)
	}
	arguments, err := method.Inputs.Pack(args...)
	if err != nil {
		return nil, err
	}
	// Pack up the method ID too if not a constructor and return
	return append(method.Id(), arguments...), nil
}

// PackMethodPanic is the panicking variant of [ABIContract.PackMethod];
// intended for build-time-known method calls where a packing failure
// is a programmer error.
func (abi ABIContract) PackMethodPanic(name string, args ...interface{}) []byte {
	data, err := abi.PackMethod(name, args...)
	if err != nil {
		panic(err)
	}
	return data
}

// PackVariable encodes args into the ABI shape of the named storage
// variable. Used by embedded contracts to write their on-chain
// records.
func (abi ABIContract) PackVariable(name string, args ...interface{}) ([]byte, error) {
	variable, exist := abi.Variables[name]
	if !exist {
		return nil, errVariableNotFound(name)
	}

	return variable.Inputs.Pack(args...)
}

// PackVariablePanic is the panicking variant of
// [ABIContract.PackVariable].
func (abi ABIContract) PackVariablePanic(name string, args ...interface{}) []byte {
	data, err := abi.PackVariable(name, args...)
	if err != nil {
		panic(err)
	}
	return data
}

// UnpackMethod decodes input (method id + arguments) into v. Returns
// an error if input is too short to carry a method id, if the id does
// not match name, or if the argument types do not match.
func (abi ABIContract) UnpackMethod(v interface{}, name string, input []byte) (err error) {
	if len(input) <= 4 {
		return errEmptyInput
	}
	if method, err := abi.MethodById(input[0:4]); err == nil && method.Name == name {
		return method.Inputs.Unpack(v, input[4:])
	}
	return errCouldNotLocateNamedMethod
}

// UnpackEmptyMethod decodes input as a no-argument method call:
// requires exactly 4 bytes (the method id) and confirms the id maps
// to name. Used to validate calls to methods with no arguments.
func (abi ABIContract) UnpackEmptyMethod(name string, input []byte) (err error) {
	if len(input) < 4 {
		return errEmptyInput
	} else if len(input) > 4 {
		return errInputTooLong
	}
	if method, err := abi.MethodById(input[0:4]); err == nil && method.Name == name {
		return nil
	}
	return errCouldNotLocateNamedMethod
}

// UnpackVariable decodes input as the ABI shape of the named variable
// into v. Used by embedded contracts to read their on-chain records.
func (abi ABIContract) UnpackVariable(v interface{}, name string, input []byte) (err error) {
	if len(input) == 0 {
		return errEmptyInput
	}
	if variable, ok := abi.Variables[name]; ok {
		return variable.Inputs.Unpack(v, input)
	}
	return errCouldNotLocateNamedVariable
}

// UnpackVariablePanic is the panicking variant of
// [ABIContract.UnpackVariable].
func (abi ABIContract) UnpackVariablePanic(v interface{}, name string, input []byte) {
	common.DealWithErr(abi.UnpackVariable(v, name, input))
}

// MethodById looks up a method by the 4-byte id. Returns nil if none
// found.
func (abi *ABIContract) MethodById(sigdata []byte) (*Method, error) {
	if len(sigdata) < 4 {
		return nil, errMethodIdNotSpecified
	}
	for _, method := range abi.Methods {
		if bytes.Equal(method.Id(), sigdata[:4]) {
			return &method, nil
		}
	}
	return nil, errNoMethodId(sigdata[:4])
}

// UnmarshalJSON implements [json.Unmarshaler]: parses the canonical
// Solidity-shaped ABI JSON ([{"type":"function","name":...,...}]).
// `function` entries become [Method]s; `variable` entries become
// [Variable]s used for storage encoding.
func (abi *ABIContract) UnmarshalJSON(data []byte) error {
	var fields []struct {
		Type    string
		Name    string
		Inputs  []Argument
		Outputs []Argument
	}

	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	abi.Methods = make(map[string]Method)
	abi.Variables = make(map[string]Variable)
	for _, field := range fields {
		switch field.Type {
		case "function":
			abi.Methods[field.Name] = newMethod(field.Name, field.Inputs)
		case "variable":
			if len(field.Inputs) == 0 {
				return errInvalidEmptyVariableInput
			}
			abi.Variables[field.Name] = Variable{
				Name:   field.Name,
				Inputs: field.Inputs,
			}
		}
	}
	return nil
}
