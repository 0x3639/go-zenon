// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

//go:build js

package server

import (
	"context"
	"errors"
	"net"
)

var errNotSupported = errors.New("rpc: not supported")

// ipcListen is a stub under //go:build js: WASM/JS targets have no
// access to OS named-pipe syscalls, so this always returns
// errNotSupported. The other build-tagged ipc_*.go files implement
// the real listen path on their respective platforms.
func ipcListen(endpoint string) (net.Listener, error) {
	return nil, errNotSupported
}

// newIPCConnection is a stub under //go:build js: see [ipcListen] for
// why this always returns errNotSupported on WASM/JS targets.
func newIPCConnection(ctx context.Context, endpoint string) (net.Conn, error) {
	return nil, errNotSupported
}
