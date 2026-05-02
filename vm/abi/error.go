package abi

import (
	"fmt"
	"math/big"
	"reflect"

	"github.com/pkg/errors"
)

// Sentinel errors returned by the ABI codec. Most carry static
// messages; the helpers below wrap dynamic context (argument names,
// type info, lengths) into formatted errors.
var (
	errBadBool                     = errors.New("abi: improperly encoded boolean value")
	errEmptyInput                  = errors.New("unmarshalling empty output")
	errInputTooLong                = errors.New("unmarshalling too long input")
	errCouldNotLocateNamedMethod   = errors.New("abi: could not locate named method")
	errCouldNotLocateNamedVariable = errors.New("abi: could not locate named variable")
	errMethodIdNotSpecified        = errors.New("method id is not specified")
	errInvalidEmptyVariableInput   = errors.New("abi: variable inputs should not be empty")
	errInvalidZeroVariableSize     = errors.New("abi: invalid zero variable size")
	errInvalidArrayTypeFormatting  = errors.New("invalid formatting of array type")
	errPackFailed                  = errors.New("abi: pack element failed")
	errPureUnderscoredOutput       = errors.New("abi: purely underscored output cannot unpack to struct")
	errInvalidlFixedBytesType      = errors.New("abi: invalid type in call to make fixed byte array")
	errInvalidlArrayType           = errors.New("abi: invalid type in array/slice unpacking stage")
)

// errArgumentJsonErr formats a JSON-decoding failure for an [Argument].
func errArgumentJsonErr(err error) error {
	return fmt.Errorf("argument json err: %v", err)
}

// errMethodNotFound is returned when [ABIContract.PackMethod] /
// [ABIContract.UnpackMethod] cannot locate name in the ABI.
func errMethodNotFound(name string) error {
	return fmt.Errorf("method '%s' not found", name)
}

// errNoMethodId is returned by [ABIContract.MethodById] when no
// method has the supplied 4-byte id.
func errNoMethodId(sigdata []byte) error {
	return fmt.Errorf("no method with id: %#x", sigdata[:4])
}

// errVariableNotFound is returned when [ABIContract.PackVariable] /
// [ABIContract.UnpackVariable] cannot locate name.
func errVariableNotFound(name string) error {
	return fmt.Errorf("varible '%s' not found", name)
}

// errType is returned when an argument's Go-side type does not match
// the ABI declaration.
func errType(expected, got interface{}) error {
	return fmt.Errorf("abi: cannot use %v as type %v as argument", got, expected)
}

// errParsingVariableSize wraps a strconv error encountered while
// parsing the size component of a `uint<N>` / `int<N>` / `bytes<N>` /
// `[<N>]` type.
func errParsingVariableSize(err error) error {
	return fmt.Errorf("abi: error parsing variable size: %v", err)
}

// errUnsupportedArgType is returned by [NewType] for a base type
// the codec does not handle.
func errUnsupportedArgType(t string) error {
	return fmt.Errorf("abi: unsupported arg type: %s", t)
}

// errUnknownType is returned by the unpacker when a parsed [Type]
// has an unrecognized .T discriminator.
func errUnknownType(t Type) error {
	return fmt.Errorf("abi: unknown type %v", t.T)
}

// errWrongPackedLength is returned when an atomic-argument unpack
// receives a multi-element marshalled values slice.
func errWrongPackedLength(marshalledValues []interface{}) error {
	return fmt.Errorf("abi: wrong length, expected single value, got %d", len(marshalledValues))
}

// errArgLengthMismatch is returned when the number of arguments the
// caller supplies does not match the ABI signature.
func errArgLengthMismatch(args []interface{}, abiArgs Arguments) error {
	return fmt.Errorf("argument count mismatch: %d for %d", len(args), len(abiArgs))
}

// errInvalidStruct is returned when a non-pointer destination is
// passed to [Arguments.Unpack].
func errInvalidStruct(v interface{}) error {
	return fmt.Errorf("abi: Unpack(non-pointer %T)", v)
}

// errInsufficientArgumentSize is returned when an unpack target
// slice/array is shorter than the number of ABI arguments.
func errInsufficientArgumentSize(arguments Arguments, value reflect.Value) error {
	return fmt.Errorf("abi: insufficient number of arguments for unpack, want %d, got %d", len(arguments), value.Len())
}

// errInsufficientElementSize is the per-array variant of
// [errInsufficientArgumentSize].
func errInsufficientElementSize(minLen int, v reflect.Value) error {
	return fmt.Errorf("abi: insufficient number of elements in the list/array for unpack, want %d, got %d",
		minLen, v.Len())
}

// errUnmarshalTypeFailed is returned when the codec cannot assign
// src to dst — incompatible Go types.
func errUnmarshalTypeFailed(src, dst reflect.Value) error {
	return fmt.Errorf("abi: cannot unmarshal %v in to %v", src.Type(), dst.Type())
}

// errInvalidTuple is returned when a tuple unpack target is neither
// struct nor slice nor array.
func errInvalidTuple(typ reflect.Type) error {
	return fmt.Errorf("abi: cannot unmarshal tuple into %v", typ)
}

// errEmptyTagName is returned when a struct field carries an empty
// `abi:""` tag.
func errEmptyTagName(structFieldName string) error {
	return fmt.Errorf("struct: abi tag in '%s' is empty", structFieldName)
}

// errTagAlreadyMapped is returned when two struct fields claim the
// same ABI tag.
func errTagAlreadyMapped(structFieldName string) error {
	return fmt.Errorf("struct: abi tag in '%s' already mapped", structFieldName)
}

// errTagNotFound is returned when an `abi:""` tag does not match any
// ABI argument.
func errTagNotFound(tagName string) error {
	return fmt.Errorf("struct: abi tag '%s' defined but not found in abi", tagName)
}

// errMultipleVariable is returned when two struct fields would map
// to the same ABI variable (one explicitly via tag, one implicitly).
func errMultipleVariable(abiFieldName string) error {
	return fmt.Errorf("abi: multiple variables maps to the same abi field '%s'", abiFieldName)
}

// errMultipleOutput is returned when two ABI fields would map to
// the same struct field.
func errMultipleOutput(structFieldName string) error {
	return fmt.Errorf("abi: multiple outputs mapping to the same struct field '%s'", structFieldName)
}

// errNegativeInputSize is returned by the unpacker for a malformed
// length prefix.
func errNegativeInputSize(size int) error {
	return fmt.Errorf("cannot marshal input to array, size is negative (%d)", size)
}

// errArrayOffsetOverflow is returned when an offset + size would
// reach beyond the available output slice.
func errArrayOffsetOverflow(output []byte, start, size int) error {
	return fmt.Errorf("abi: cannot marshal in to go array: offset %d would go over slice boundary (len=%d)", len(output), start+WordSize*size)
}

// errInsufficientLength is returned when output is shorter than
// `index + WordSize`.
func errInsufficientLength(outputSize []byte, index int) error {
	return fmt.Errorf("abi: cannot marshal in to go type: length insufficient %d require %d", outputSize, index+WordSize)
}

// errBigSliceOffsetOverflow is returned by [lengthPrefixPointsTo]
// when the encoded offset exceeds the output length.
func errBigSliceOffsetOverflow(bigOffsetEnd, outputLength *big.Int) error {
	return fmt.Errorf("abi: cannot marshal in to go slice: offset %v would go over slice boundary (len=%v)", bigOffsetEnd, outputLength)
}

// errBigOffsetOverflow is returned when the encoded offset does not
// fit in an int64.
func errBigOffsetOverflow(bigOffsetEnd *big.Int) error {
	return fmt.Errorf("abi offset larger than int64: %v", bigOffsetEnd)
}

// errBigLengthOverflow is returned when the encoded length does not
// fit in an int64.
func errBigLengthOverflow(totalSize *big.Int) error {
	return fmt.Errorf("abi length larger than int64: %v", totalSize)
}

// errInsufficientBigLength is the [big.Int] variant of
// [errInsufficientLength].
func errInsufficientBigLength(outputLength, totalSize *big.Int) error {
	return fmt.Errorf("abi: cannot marshal in to go type: length insufficient %v require %v", outputLength, totalSize)
}

// formatSliceString formats the reflection kind with the given slice
// size and returns a formatted string representation.
func formatSliceString(kind reflect.Kind, sliceSize int) string {
	if sliceSize == -1 {
		return fmt.Sprintf("[]%v", kind)
	}
	return fmt.Sprintf("[%d]%v", sliceSize, kind)
}

// sliceTypeCheck checks that the given slice can be assigned to the
// reflection type in t. Recursive: validates element type, nested
// slice/array shapes, and per-element kind compatibility.
func sliceTypeCheck(t Type, val reflect.Value) error {
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return errType(formatSliceString(t.Kind, t.Size), val.Type())
	}

	if t.T == ArrayTy && val.Len() != t.Size {
		return errType(formatSliceString(t.Elem.Kind, t.Size), formatSliceString(val.Type().Elem().Kind(), val.Len()))
	}

	if t.Elem.T == SliceTy {
		if val.Len() > 0 {
			return sliceTypeCheck(*t.Elem, val.Index(0))
		}
	} else if t.Elem.T == ArrayTy {
		return sliceTypeCheck(*t.Elem, val.Index(0))
	}

	if elemKind := val.Type().Elem().Kind(); (elemKind != reflect.Slice && elemKind != t.Elem.Kind) ||
		(elemKind == reflect.Slice && t.Elem.Kind != reflect.Slice && t.Elem.Kind != reflect.Array) ||
		(elemKind != reflect.Slice && elemKind != reflect.Array && t.Elem.Kind == reflect.Array) {
		return errType(formatSliceString(t.Elem.Kind, t.Size), val.Type())
	}
	return nil
}

// typeCheck checks that the given reflection value can be assigned
// to the reflection type in t. For slice/array types it dispatches
// to [sliceTypeCheck]; for fixed-byte types it confirms the length;
// for everything else it compares the [reflect.Kind].
func typeCheck(t Type, value reflect.Value) error {
	if t.T == SliceTy || t.T == ArrayTy {
		return sliceTypeCheck(t, value)
	}

	// Check base type validity. Element types will be checked later on.
	if t.Kind != reflect.Array && t.Kind != value.Kind() {
		return errType(t.Kind, value.Kind())
	} else if t.T == FixedBytesTy && t.Size != value.Len() {
		return errType(t.Type, value.Type())
	} else {
		return nil
	}

}
