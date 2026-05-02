package abi

import (
	"fmt"
	"strings"

	"github.com/zenon-network/go-zenon/common/types"
)

// Method is one ABI function entry: its name, input arguments, and
// the 4-byte method id (first four bytes of the canonical signature
// hash) used as the call's selector in [nom.AccountBlock.Data].
type Method struct {
	Name   string
	id     []byte
	Inputs Arguments
}

// newMethod builds a [Method] and pre-computes its 4-byte id by
// hashing the canonical signature.
func newMethod(name string, inputs Arguments) Method {
	m := Method{
		Name:   name,
		Inputs: inputs,
	}
	m.id = types.NewHash([]byte(m.Sig())).Bytes()[:4]
	return m
}

// Sig returns the canonical Solidity-style signature
// (`name(type1,type2,...)`). The hash of this string is the source of
// the 4-byte method id.
func (method Method) Sig() string {
	types := make([]string, len(method.Inputs))
	for i, input := range method.Inputs {
		types[i] = input.Type.String()
	}
	return fmt.Sprintf("%v(%v)", method.Name, strings.Join(types, ","))
}

// String returns a human-readable representation
// (`onMessage name(type arg, type arg, ...)`). Used in log lines.
func (method Method) String() string {
	inputs := make([]string, len(method.Inputs))
	for i, input := range method.Inputs {
		inputs[i] = fmt.Sprintf("%v %v", input.Type, input.Name)
	}
	return fmt.Sprintf("onMessage %v(%v)", method.Name, strings.Join(inputs, ", "))
}

// Id returns the 4-byte method id (the call selector).
func (method Method) Id() []byte {
	return method.id
}
