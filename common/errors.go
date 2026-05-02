package common

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/inconshreveable/log15"
	"github.com/pkg/errors"

	"github.com/zenon-network/go-zenon/rpc/server"
)

// NewErrorWCode returns a coded error suitable for return from RPC
// handlers. The integer code is surfaced to JSON-RPC clients via the
// [server.Error] interface.
func NewErrorWCode(code int, errStr string) ErrorWCode {
	return &errorWCode{
		error: errors.New(errStr),
		code:  code,
	}
}

// ErrorWCode is the project's error-with-code interface for RPC handlers.
// Implementations satisfy [server.Error] and additionally allow appending
// a free-form detail string.
type ErrorWCode interface {
	server.Error
	// AddDetail returns a copy of the error with detail appended to the
	// underlying message. The original error is preserved for unwrapping
	// via `errors.Is` / `errors.Unwrap`.
	AddDetail(detail string) ErrorWCode
}

// errorWCode is the canonical [ErrorWCode] implementation.
type errorWCode struct {
	error
	code int
}

// ErrorCode returns the integer code carried by the error.
func (err *errorWCode) ErrorCode() int {
	return err.code
}

// AddDetail returns a new error sharing the same code with detail wrapped
// onto the message; the original error is reachable through unwrap.
func (err *errorWCode) AddDetail(detail string) ErrorWCode {
	return &errorWCode{
		code:  err.code,
		error: fmt.Errorf("%w;%v", err.error, detail),
	}
}

// T is the minimal subset of [testing.T] consumed by the assertion helpers
// in this file. Lifting it to a separate interface lets the helpers be used
// from tests in any package without dragging in the full testing package.
type T interface {
	Fatalf(format string, args ...interface{})
	TempDir() string
}

// DealWithErr panics if v is not nil. Used at boundaries where the caller
// has chosen to treat errors as bugs (e.g., serializing a fixed-shape
// protobuf). Stack-traced via [RecoverStack] when the panic propagates.
func DealWithErr(v interface{}) {
	defer RecoverStack()
	if v != nil {
		panic(v)
	}
}

// RecoverStack is a deferred handler that logs the panic value with a
// captured stack trace and then re-panics to preserve the original
// behavior. Used to make panics observable in production logs.
func RecoverStack() {
	if err := recover(); err != nil {
		var e error
		switch t := err.(type) {
		case error:
			e = errors.WithStack(t)
		case string:
			e = errors.New(t)
		default:
			e = errors.Errorf("unknown type %+v", err)
		}

		log15.Error("panic", "err", err, "withstack", e)
		fmt.Printf("%+v", e)
		panic(err)
	}
}

// Stack-frame filtering for [GetStack]. The test-helper machinery walks the
// call stack looking for the first frame in a `_test.go` file so test
// failures point at the test, not at this helper.
var (
	includeFiles = []string{
		"_test.go",
	}
	excludeFiles = []string{}
)

// trimEol strips leading and trailing newlines from a so multi-line
// expected/received strings line up cleanly when diffed.
func trimEol(a string) string {
	for len(a) != 0 && a[0] == '\n' {
		a = a[1:]
	}
	for len(a) != 0 && a[len(a)-1] == '\n' {
		a = a[0 : len(a)-1]
	}
	return a
}

// expectError formats a side-by-side expected/received diff with a stack
// pointer, then fails the test through t.Fatalf.
func expectError(t T, received, expected, stack string) {
	expected = trimEol(expected)
	received = trimEol(received)
	t.Fatalf("\n<<<<<<< Expected\n%v\n=======\n%v\n>>>>>>> Received\n%v\n", expected, received, stack)
}

// expectString fails the test if current and expected differ after newline
// trimming; intended for use under [Expecter].
func expectString(t T, current, expected, stack string) {
	current = trimEol(current)
	expected = trimEol(expected)
	if current != expected {
		expectError(t, current, expected, stack)
	}
}

// GetStack returns the test-file frame nearest the top of the current
// stack, suitable for embedding in a test failure message. Falls back to
// the top frame if no test frame is found.
func GetStack() string {
	st := string(debug.Stack())
	frames := strings.Split(st, "\n")
	for i := 2; i < len(frames); i += 2 {
		ok := false
		for _, file := range includeFiles {
			if strings.Contains(frames[i], file) {
				ok = true
			}
		}

		if !ok {
			continue
		}

		for _, file := range excludeFiles {
			if strings.Contains(frames[i], file) {
				ok = false
			}
		}

		if ok {
			return frames[i]
		}
	}
	return frames[0]
}

// FailIfErr fails the test through t.Fatalf if err is non-nil, attaching
// a [GetStack] pointer.
func FailIfErr(t T, err error) {
	if err != nil {
		t.Fatalf("'%v'\n%v", err, GetStack())
	}
}

// ExpectError asserts that current equals expected (by string form). Used
// in tests to verify that a function returns a specific sentinel error.
func ExpectError(t T, current error, expected error) {
	if current != expected {
		expectError(t, fmt.Sprintf("%v", current), fmt.Sprintf("%v", expected), GetStack())
	}
}

// ExpectBytes asserts that current encoded with `0x`-prefix hex equals expected.
func ExpectBytes(t T, current []byte, expected string) {
	ExpectString(t, hexutil.Encode(current), expected)
}

// ExpectTrue asserts that value is true.
func ExpectTrue(t T, value bool) {
	if !value {
		expectError(t, "False", "True", GetStack())
	}
}

// ExpectUint64 asserts that current equals expected.
func ExpectUint64(t T, current, expected uint64) {
	if current != expected {
		expectError(t, fmt.Sprintf("%v", current), fmt.Sprintf("%v", expected), GetStack())
	}
}

// ExpectAmount asserts that current equals expected by [big.Int.Cmp].
func ExpectAmount(t T, current, expected *big.Int) {
	if current.Cmp(expected) != 0 {
		expectError(t, current.String(), expected.String(), GetStack())
	}
}

// ExpectString asserts that current equals expected after newline trimming.
func ExpectString(t T, current, expected string) {
	current = trimEol(current)
	expected = trimEol(expected)
	if current != expected {
		expectError(t, current, expected, GetStack())
	}
}

// ExpectJson asserts that the indented JSON of current equals expected.
func ExpectJson(t T, current interface{}, expected string) {
	strBytes, err := json.MarshalIndent(current, "", "\t")
	FailIfErr(t, err)
	ExpectString(t, string(strBytes), expected)
}

// Expect asserts that current and expected stringify to the same value.
// Generic form of [ExpectString] for non-string types.
func Expect(t T, current, expected interface{}) {
	currentStr := trimEol(fmt.Sprintf("%v", current))
	expectedStr := trimEol(fmt.Sprintf("%v", expected))
	if currentStr != expectedStr {
		expectError(t, currentStr, expectedStr, GetStack())
	}
}

// Expecter accumulates a test value (string, JSON, or a deferred callback)
// and provides assertion methods that report failure with the captured
// call stack. Builders such as [Expecter.HideHashes] and
// [Expecter.SubJson] mutate the expecter in place and return it so chains
// read naturally.
type Expecter struct {
	hideHash bool
	subJson  interface{}

	receivedF   func() (string, error)
	received    string
	receivedErr error
	stack       string
}

// String wraps received in an [Expecter]. Used when the caller already
// has the value in string form.
func String(received string) *Expecter {
	return &Expecter{
		hideHash:    false,
		received:    received,
		receivedErr: nil,
	}
}

// Json wraps a JSON-encodable j in an [Expecter]. Pass inheritedError to
// surface an error encountered while producing j.
func Json(j interface{}, inheritedError error) *Expecter {
	receivedBytes, err := json.MarshalIndent(j, "", "\t")
	DealWithErr(err)
	return &Expecter{
		received:    string(receivedBytes),
		receivedErr: inheritedError,
	}
}

// LateCaller defers the production of the received value until [Expecter.Equals]
// or [Expecter.Error] is called. Used by [SaveLogs] so the captured log
// content reflects everything written up to assertion time.
func LateCaller(f func() (string, error)) *Expecter {
	return &Expecter{
		receivedF: f,
		stack:     GetStack(),
	}
}

// HideHashes replaces every 64-character hex run with a stable placeholder
// and every 86-character base64 signature with another placeholder. Used
// to make test golden output stable across runs.
func HideHashes(a string) string {
	a = regexp.MustCompile(`[0-9a-f]{64}`).ReplaceAllString(a, "XXXHASHXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX")
	a = regexp.MustCompile(`[A-Za-z0-9+/]{86}==`).ReplaceAllString(a, "XXXSIGNATUREXINXBASEX64XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX")
	return a
}

// HideHashes enables hash/signature redaction on this expecter; the
// transformation runs at assertion time.
func (exp *Expecter) HideHashes() *Expecter {
	exp.hideHash = true
	return exp
}

// SubJson decodes the received JSON into subJson before stringifying again.
// Used to assert on a typed subset of a larger JSON payload.
func (exp *Expecter) SubJson(subJson interface{}) *Expecter {
	exp.subJson = subJson
	return exp
}

// Equals asserts that the received value (after deferred resolution,
// optional sub-JSON projection, and optional hash redaction) equals expected.
// Fails the test through t.Fatalf if it does not.
func (exp *Expecter) Equals(t T, expected string) {
	received := exp.received
	if exp.receivedF != nil {
		received, exp.receivedErr = exp.receivedF()
	}
	if exp.receivedErr != nil {
		t.Fatalf("got error '%v' when expecting a clean execution", exp.receivedErr)
	}
	if exp.subJson != nil && received != "null" {
		err := json.Unmarshal([]byte(received), exp.subJson)
		FailIfErr(t, err)
		receivedBytes, err := json.MarshalIndent(exp.subJson, "", "\t")
		FailIfErr(t, err)
		received = string(receivedBytes)
	}
	if exp.hideHash {
		received = HideHashes(received)
	}
	if exp.stack == "" {
		exp.stack = GetStack()
	}
	expectString(t, received, expected, exp.stack)
}

// Error asserts that the deferred receiver returned an error matching err
// (by string form).
func (exp *Expecter) Error(t T, err error) {
	if exp.receivedF != nil {
		_, exp.receivedErr = exp.receivedF()
	}
	received := fmt.Sprintf("%v", exp.receivedErr)
	if exp.stack == "" {
		exp.stack = GetStack()
	}
	expectString(t, received, fmt.Sprintf("%v", err), exp.stack)
}
