package gokux_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/dengliu/gokux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// discardLogger returns a logger that produces no output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRun_BindError_ReturnsImmediately(t *testing.T) {
	// Occupy a port on all interfaces so Echo's ":port" bind will conflict.
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	port := ln.Addr().(*net.TCPAddr).Port

	app := gokux.New(
		gokux.WithLogger(discardLogger()),
	)
	require.NoError(t, app.Init())

	// Override the config port to the already-bound port.
	app.Config.Server.Port = port

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(context.Background())
	}()

	select {
	case runErr := <-errCh:
		require.Error(t, runErr, "Run should return an error when the port is already bound")
		assert.Contains(t, runErr.Error(), "server start")
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5 seconds; the bind error was likely swallowed")
	}
}

func TestRun_GracefulShutdown_ReturnsNil(t *testing.T) {
	app := gokux.New(
		gokux.WithLogger(discardLogger()),
	)
	require.NoError(t, app.Init())

	// Use port 0 so the OS picks a free port.
	app.Config.Server.Port = 0
	app.Config.Server.DrainWaitSeconds = 0
	app.Config.Server.ShutdownTimeoutSeconds = 5

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(ctx)
	}()

	// Give the server time to start.
	time.Sleep(200 * time.Millisecond)

	// Cancel the context to trigger graceful shutdown.
	cancel()

	select {
	case runErr := <-errCh:
		assert.NoError(t, runErr, "Run should return nil on graceful shutdown")
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return within 10 seconds after context cancellation")
	}
}

func TestRun_BindError_WrapsOriginalError(t *testing.T) {
	// Occupy a port on all interfaces so Echo's ":port" bind will conflict.
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	port := ln.Addr().(*net.TCPAddr).Port

	app := gokux.New(
		gokux.WithLogger(discardLogger()),
	)
	require.NoError(t, app.Init())
	app.Config.Server.Port = port

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(context.Background())
	}()

	select {
	case runErr := <-errCh:
		require.Error(t, runErr)
		// The original bind error should mention "address already in use" or "bind".
		errMsg := strings.ToLower(runErr.Error())
		assert.True(t,
			strings.Contains(errMsg, "address already in use") || strings.Contains(errMsg, "bind"),
			"expected bind-related error, got: %s", runErr.Error(),
		)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5 seconds")
	}
}
