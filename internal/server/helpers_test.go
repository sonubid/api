package server_test

import (
	"log/slog"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// discardLogger returns a logger that discards all output, suitable for tests.
func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// freeAddr returns a TCP address with an available ephemeral port.
func freeAddr(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", ":0") //nolint:gosec,noctx // test-only: binding to all interfaces is intentional
	require.NoError(t, err)

	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	return addr
}

// waitForServer polls addr until a TCP connection is accepted or the wait
// timeout is exceeded, at which point it fails the test.
func waitForServer(t *testing.T, addr string) {
	t.Helper()

	require.Eventually(t, func() bool {
		conn, err := net.Dial("tcp", addr) //nolint:noctx // test-only convenience
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, waitTimeout, pollInterval, "server did not start listening on %s", addr)
}
