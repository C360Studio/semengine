package service

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
)

// TestObservedManagerHTTPURLReachesTheManager is #372's regression. The manager binds its main
// listener on every address (":port"). On macOS a second socket may then bind 127.0.0.1 on the
// same port and takes every connection made to 127.0.0.1, so a URL built from 127.0.0.1 can reach
// another server (a concurrent package's httptest server, for one). The test plants such a
// server; the helper must still reach the manager. Where the OS refuses the second bind (Linux),
// the plant is absent and the test checks the plain case.
func TestObservedManagerHTTPURLReachesTheManager(t *testing.T) {
	manager := createTestServiceManager(ManagerConfig{}, testComponentDependencies(t, component.NewRegistry()))
	require.NoError(t, RegisterAll(manager.registry))
	startTestManager(t, manager)

	manager.mu.RLock()
	port := manager.httpListener.Addr().(*net.TCPAddr).Port
	manager.mu.RUnlock()
	if planted, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port))); err == nil {
		server := &http.Server{Handler: http.NotFoundHandler()}
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = server.Serve(planted)
		}()
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
			defer cancel()
			require.NoError(t, server.Shutdown(ctx))
			<-done
		})
	}

	requireAnswer(t, observedManagerHTTPURL(t, manager)+"/healthz", http.StatusOK, "")
}
