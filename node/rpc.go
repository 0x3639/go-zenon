package node

// startRPC configures and launches the HTTP and WebSocket RPC
// transports. Called once during [Node.Start]; not safe to call
// after the node is running because it assumes the listener and
// handler atomic.Values are uninitialised.
func (node *Node) startRPC() error {
	// Configure HTTP.
	if node.config.RPC.HTTPHost != "" {
		config := httpConfig{
			CorsAllowedOrigins: node.config.RPC.HTTPCors,
			Vhosts:             node.config.RPC.HTTPVirtualHosts,
			Modules:            node.config.RPC.Endpoints,
			prefix:             "",
		}
		if err := node.http.setListenAddr(node.config.RPC.HTTPHost, node.config.RPC.HTTPPort); err != nil {
			return err
		}
		if err := node.http.enableRPC(node.rpcAPIs, config); err != nil {
			return err
		}
	}

	// Configure WebSocket.
	if node.config.RPC.WSHost != "" {
		server := node.wsServerForPort(node.config.RPC.WSPort)
		config := wsConfig{
			Modules: node.config.RPC.Endpoints,
			Origins: node.config.RPC.WSOrigins,
			prefix:  "",
		}
		if err := server.setListenAddr(node.config.RPC.WSHost, node.config.RPC.WSPort); err != nil {
			return err
		}
		if err := server.enableWS(node.rpcAPIs, config); err != nil {
			return err
		}
	}

	if err := node.http.start(); err != nil {
		return err
	}
	return node.ws.start()
}

// wsServerForPort returns the [httpServer] that should host the
// WebSocket transport for the given port — the same server as HTTP
// when the ports collide (or HTTP is disabled), otherwise the
// dedicated ws instance.
func (node *Node) wsServerForPort(port int) *httpServer {
	if node.config.RPC.HTTPHost == "" || node.http.port == port {
		return node.http
	}
	return node.ws
}

// stopRPC shuts down both transports. Safe to call against
// already-stopped servers.
func (node *Node) stopRPC() {
	node.http.stop()
	node.ws.stop()
}
