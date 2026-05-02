package abi

import (
	"fmt"
	"strings"
)

// Variable represents an embedded-contract storage shape, encoded
// the same way as a method's argument tuple but used for ABI-encoded
// contract storage records rather than method calls.
//
// Variable is used only in built-in contracts.
type Variable struct {
	Name   string
	Inputs Arguments
}

// String returns a struct-style summary
// (`struct Name { type field, ... }`) used in log lines.
func (v Variable) String() string {
	inputs := make([]string, len(v.Inputs))
	for i, input := range v.Inputs {
		inputs[i] = fmt.Sprintf("%v %v", input.Type, input.Name)
	}
	return fmt.Sprintf("struct %v { %v }", v.Name, strings.Join(inputs, ","))
}
