package node

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/inconshreveable/log15"

	rpc "github.com/zenon-network/go-zenon/rpc/server"
)

// probeService is the only API registered by these tests; it exists so a
// successful JSON-RPC round trip proves the handler is live.
type probeService struct{}

func (probeService) Ping() string { return "pong" }

const pingRequest = `{"jsonrpc":"2.0","id":7,"method":"probe.ping","params":[]}`

// pingResponse is the subset of a JSON-RPC response the tests check.
type pingResponse struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

// checkPong fails the test unless raw is a successful reply to pingRequest:
// same id, "pong" result, no error.
func checkPong(t *testing.T, what string, raw []byte) {
	t.Helper()
	var resp pingResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("%s: undecodable response %q: %v", what, raw, err)
	}
	if string(resp.ID) != "7" {
		t.Fatalf("%s: response id = %s, want 7", what, resp.ID)
	}
	if len(resp.Error) != 0 && string(resp.Error) != "null" {
		t.Fatalf("%s: unexpected RPC error %s", what, resp.Error)
	}
	if string(resp.Result) != `"pong"` {
		t.Fatalf("%s: result = %s, want \"pong\"", what, resp.Result)
	}
}

// newRPCTestNode builds the node state startRPC works on, without the
// wallet, chain or p2p server.
func newRPCTestNode(cfg RPCConfig) *Node {
	return &Node{
		config:  &Config{RPC: cfg},
		http:    newHTTPServer(rpc.DefaultHTTPTimeouts),
		ws:      newHTTPServer(rpc.DefaultHTTPTimeouts),
		rpcAPIs: []rpc.API{{Namespace: "probe", Service: probeService{}, Public: true}},
	}
}

func startRPCTestNode(t *testing.T, cfg RPCConfig) *Node {
	t.Helper()
	n := newRPCTestNode(cfg)
	if err := n.startRPC(); err != nil {
		t.Fatalf("startRPC: %v", err)
	}
	t.Cleanup(n.stopRPC)
	return n
}

// startRPCTestNodeOnFreePorts is for configurations that need a specific
// non-zero port. Such a port can only be chosen by binding and releasing
// it, so another process may take it before startRPC binds it again; an
// address-in-use failure is retried with a fresh configuration.
func startRPCTestNodeOnFreePorts(t *testing.T, build func() RPCConfig) *Node {
	t.Helper()
	for attempt := 0; attempt < 10; attempt++ {
		n := newRPCTestNode(build())
		err := n.startRPC()
		if err == nil {
			t.Cleanup(n.stopRPC)
			return n
		}
		n.stopRPC()
		if !errors.Is(err, syscall.EADDRINUSE) && !strings.Contains(err.Error(), "address already in use") {
			t.Fatalf("startRPC: %v", err)
		}
	}
	t.Fatal("startRPC: chosen ports were taken on every attempt")
	return nil
}

// syncBuffer collects log output from any goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureNodeLog routes the node logger into a buffer for the rest of the
// test and restores the previous handler afterwards.
func captureNodeLog(t *testing.T) *syncBuffer {
	t.Helper()
	buf := new(syncBuffer)
	prev := log.GetHandler()
	log.SetHandler(log15.StreamHandler(buf, log15.LogfmtFormat()))
	t.Cleanup(func() { log.SetHandler(prev) })
	return buf
}

// requireDisabledLog asserts that the "disabled by configuration" line for a
// protocol, naming the ignored host, is present exactly once or absent.
func requireDisabledLog(t *testing.T, logs, protocol, host string, want bool) {
	t.Helper()
	msg := `msg="` + protocol + `-RPC server disabled by configuration"`
	got := 0
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, msg) && strings.Contains(line, "ignored-host="+host) {
			got++
		}
	}
	if (got == 1) != want || got > 1 {
		t.Fatalf("%s disabled-by-configuration log lines=%d, want present=%v; logs:\n%s", protocol, got, want, logs)
	}
}

// boundAddr returns the address a server is listening on, or "" when it has
// no listener.
func boundAddr(h *httpServer) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.listener == nil {
		return ""
	}
	return h.listener.Addr().String()
}

// freePort returns a port that was free at the time of the call. The
// listener is closed before returning, so callers use it for the port
// number only.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// httpPost sends a JSON-RPC ping over plain HTTP and returns the status code
// and body. It fails the test on a transport error.
func httpPost(t *testing.T, addr string) (int, []byte) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post("http://"+addr, "application/json", strings.NewReader(pingRequest))
	if err != nil {
		t.Fatalf("POST %s: %v", addr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response from %s: %v", addr, err)
	}
	return resp.StatusCode, body
}

// requireHTTPPing asserts that a JSON-RPC ping over HTTP round-trips on addr.
func requireHTTPPing(t *testing.T, addr string) {
	t.Helper()
	status, body := httpPost(t, addr)
	if status != http.StatusOK {
		t.Fatalf("HTTP RPC on %s: status %d, body %q", addr, status, body)
	}
	checkPong(t, "HTTP RPC on "+addr, body)
}

// requireNoHTTPRPC asserts that a listener refuses plain HTTP JSON-RPC.
func requireNoHTTPRPC(t *testing.T, addr string) {
	t.Helper()
	status, body := httpPost(t, addr)
	if status != http.StatusNotFound {
		t.Fatalf("HTTP RPC on %s should be unavailable, got status %d, body %q", addr, status, body)
	}
}

// wsDial attempts a WebSocket handshake with addr and returns the connection,
// or nil when the handshake is refused.
func wsDial(addr string) *websocket.Conn {
	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, _, err := dialer.Dial("ws://"+addr, nil)
	if err != nil {
		return nil
	}
	return conn
}

// requireWSPing asserts that a JSON-RPC ping over WebSocket round-trips on addr.
func requireWSPing(t *testing.T, addr string) {
	t.Helper()
	conn := wsDial(addr)
	if conn == nil {
		t.Fatalf("WS handshake refused on %s", addr)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(pingRequest)); err != nil {
		t.Fatalf("WS write on %s: %v", addr, err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("WS set read deadline on %s: %v", addr, err)
	}
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("WS read on %s: %v", addr, err)
	}
	checkPong(t, "WS RPC on "+addr, msg)
}

// requireNoWS asserts that a listener refuses the WebSocket upgrade.
func requireNoWS(t *testing.T, addr string) {
	t.Helper()
	if conn := wsDial(addr); conn != nil {
		_ = conn.Close()
		t.Fatalf("listener %s upgrades WebSocket connections but should not", addr)
	}
}

// requireUnbound asserts that a server has neither a listener nor a
// configured endpoint.
func requireUnbound(t *testing.T, what string, h *httpServer) {
	t.Helper()
	if a := boundAddr(h); a != "" || h.listenAddr() != "" {
		t.Fatalf("%s should not be bound, got listener %q endpoint %q", what, a, h.listenAddr())
	}
}

// A protocol whose enable flag is false must not bind a socket even though
// its host is configured; the flag is the policy and the host only says
// where an enabled protocol listens. Both-enabled uses the same port for
// both protocols, which exercises the shared-listener path. A protocol that
// is disabled while its host is configured is reported once in the log.
func TestStartRPCHonorsEnableFlags(t *testing.T) {
	cases := []struct {
		name                 string
		enableHTTP, enableWS bool
	}{
		{"both disabled", false, false},
		{"http only", true, false},
		{"ws only", false, true},
		{"both enabled", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureNodeLog(t)
			n := startRPCTestNode(t, RPCConfig{
				EnableHTTP: tc.enableHTTP,
				HTTPHost:   "127.0.0.1",
				HTTPPort:   0,
				EnableWS:   tc.enableWS,
				WSHost:     "127.0.0.1",
				WSPort:     0,
			})

			switch {
			case tc.enableHTTP && tc.enableWS:
				// Same port on both: WebSocket shares the HTTP server and the
				// dedicated WebSocket server stays unbound.
				addr := boundAddr(n.http)
				requireHTTPPing(t, addr)
				requireWSPing(t, addr)
				requireUnbound(t, "dedicated WS server", n.ws)

			case tc.enableHTTP:
				addr := boundAddr(n.http)
				requireHTTPPing(t, addr)
				requireNoWS(t, addr)
				requireUnbound(t, "WS server", n.ws)

			case tc.enableWS:
				addr := boundAddr(n.ws)
				requireWSPing(t, addr)
				requireNoHTTPRPC(t, addr)
				requireUnbound(t, "HTTP server", n.http)

			default:
				requireUnbound(t, "HTTP server", n.http)
				requireUnbound(t, "WS server", n.ws)
			}
			requireDisabledLog(t, logs.String(), "HTTP", "127.0.0.1", !tc.enableHTTP)
			requireDisabledLog(t, logs.String(), "WS", "127.0.0.1", !tc.enableWS)
		})
	}
}

// The disabled-by-configuration line is only about a configured host being
// ignored: a protocol that is off because its host is empty is not reported.
func TestStartRPCEmptyHostIsNotReportedAsDisabled(t *testing.T) {
	logs := captureNodeLog(t)
	startRPCTestNode(t, RPCConfig{
		EnableHTTP: false, HTTPHost: "", HTTPPort: 0,
		EnableWS: false, WSHost: "", WSPort: 0,
	})
	if got := logs.String(); strings.Contains(got, "disabled by configuration") {
		t.Fatalf("empty hosts reported as disabled by configuration:\n%s", got)
	}
}

// With both protocols enabled on different ports each gets its own listener,
// and neither listener serves the other protocol.
func TestStartRPCSeparatePorts(t *testing.T) {
	n := startRPCTestNodeOnFreePorts(t, func() RPCConfig {
		return RPCConfig{
			EnableHTTP: true, HTTPHost: "127.0.0.1", HTTPPort: 0,
			EnableWS: true, WSHost: "127.0.0.1", WSPort: freePort(t),
		}
	})

	httpAddr, wsAddr := boundAddr(n.http), boundAddr(n.ws)
	if httpAddr == "" || wsAddr == "" {
		t.Fatalf("expected two listeners, got http %q ws %q", httpAddr, wsAddr)
	}
	if httpAddr == wsAddr {
		t.Fatalf("HTTP and WS share listener %s despite different ports", httpAddr)
	}

	requireHTTPPing(t, httpAddr)
	requireNoWS(t, httpAddr)

	requireWSPing(t, wsAddr)
	requireNoHTTPRPC(t, wsAddr)
}

// With both protocols enabled on the same non-zero port, as in a deployed
// configuration, WebSocket shares the HTTP listener on that port and the
// dedicated WebSocket server stays unbound.
func TestStartRPCSharedNonZeroPort(t *testing.T) {
	var port int
	n := startRPCTestNodeOnFreePorts(t, func() RPCConfig {
		port = freePort(t)
		return RPCConfig{
			EnableHTTP: true, HTTPHost: "127.0.0.1", HTTPPort: port,
			EnableWS: true, WSHost: "127.0.0.1", WSPort: port,
		}
	})

	addr := boundAddr(n.http)
	if want := net.JoinHostPort("127.0.0.1", strconv.Itoa(port)); addr != want {
		t.Fatalf("HTTP listener on %q, want %q", addr, want)
	}
	requireHTTPPing(t, addr)
	requireWSPing(t, addr)
	requireUnbound(t, "dedicated WS server", n.ws)
}

// The existing empty-host behavior is unchanged: no host, no listener, even
// with the flag set. An empty host on one protocol does not affect the other.
func TestStartRPCEmptyHostStaysDisabled(t *testing.T) {
	cases := []struct {
		name             string
		httpHost, wsHost string
	}{
		{"both empty", "", ""},
		{"http only", "127.0.0.1", ""},
		{"ws only", "", "127.0.0.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := startRPCTestNode(t, RPCConfig{
				EnableHTTP: true, HTTPHost: tc.httpHost, HTTPPort: 0,
				EnableWS: true, WSHost: tc.wsHost, WSPort: 0,
			})

			if tc.httpHost == "" {
				requireUnbound(t, "HTTP server", n.http)
			} else {
				requireHTTPPing(t, boundAddr(n.http))
			}
			if tc.wsHost == "" {
				requireUnbound(t, "WS server", n.ws)
			} else {
				requireWSPing(t, boundAddr(n.ws))
			}
		})
	}
}

// A disabled protocol must not claim its port either, whether or not the
// other protocol is enabled. The disabled protocol's port is held by another
// listener for the whole of startRPC: had it been bound, startRPC would have
// failed with an address-in-use error.
func TestDisabledProtocolLeavesPortFree(t *testing.T) {
	cases := []struct {
		name                 string
		enableHTTP, enableWS bool
	}{
		{"both disabled", false, false},
		{"http enabled, ws disabled", true, false},
		{"ws enabled, http disabled", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = l.Close() }()
			held := l.Addr().(*net.TCPAddr).Port

			// An enabled protocol takes an ephemeral port; a disabled one is
			// configured on the held port.
			cfg := RPCConfig{
				EnableHTTP: tc.enableHTTP, HTTPHost: "127.0.0.1", HTTPPort: held,
				EnableWS: tc.enableWS, WSHost: "127.0.0.1", WSPort: held,
			}
			if tc.enableHTTP {
				cfg.HTTPPort = 0
			}
			if tc.enableWS {
				cfg.WSPort = 0
			}
			n := startRPCTestNode(t, cfg)

			if tc.enableHTTP {
				requireHTTPPing(t, boundAddr(n.http))
			} else {
				requireUnbound(t, "HTTP server", n.http)
			}
			if tc.enableWS {
				requireWSPing(t, boundAddr(n.ws))
			} else {
				requireUnbound(t, "WS server", n.ws)
			}
		})
	}
}
