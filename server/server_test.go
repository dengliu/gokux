package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	slogecho "github.com/samber/slog-echo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/dengliu/gokux/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Port:                   8080,
			ShutdownTimeoutSeconds: 5,
			DrainWaitSeconds:       0, // zero drain for fast tests
		},
		Log: config.LogConfig{
			Level: "info",
		},
	}
}

func testLogger() *slog.Logger {
	return slog.Default()
}

func TestNewServer(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()

	srv, err := NewServer(cfg, logger, nil)
	require.NoError(t, err)

	assert.NotNil(t, srv.Echo)
	assert.Equal(t, cfg, srv.config)
	assert.Equal(t, logger, srv.logger)
	assert.False(t, srv.ready.Load(), "server should start in not-ready state")
	assert.NotNil(t, srv.health)
	assert.NotNil(t, srv.metrics)
}

func TestNewServer_DefaultNotReady(t *testing.T) {
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil)
	require.NoError(t, err)

	// Before SetReady(true), /readyz should return 503.
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "/readyz should be 503 before SetReady")

	// After SetReady(true), /readyz should return 200.
	srv.SetReady(true)
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec = httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "/readyz should be 200 after SetReady")
}

func TestNewServer_BuiltInRoutes(t *testing.T) {
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil)
	require.NoError(t, err)

	// Mark ready so /readyz returns 200 in the route test.
	srv.SetReady(true)

	tests := []struct {
		path       string
		wantStatus int
		wantKey    string
		wantValue  string
	}{
		{"/healthz", http.StatusOK, "status", "alive"},
		{"/readyz", http.StatusOK, "status", "ready"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			srv.Echo.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			var body map[string]string
			err := json.Unmarshal(rec.Body.Bytes(), &body)
			require.NoError(t, err)
			assert.Equal(t, tt.wantValue, body[tt.wantKey])
		})
	}
}

func TestNewServer_MetricsRoute(t *testing.T) {
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "go_goroutines")
}

func TestServer_StartAndShutdown(t *testing.T) {
	cfg := testConfig()
	cfg.Server.Port = 0 // let OS pick a free port
	srv, err := NewServer(cfg, testLogger(), nil)
	require.NoError(t, err)

	// Start in background.
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Give the server a moment to start.
	time.Sleep(100 * time.Millisecond)

	// Shutdown should succeed.
	err = srv.Shutdown(5 * time.Second)
	require.NoError(t, err)
	assert.False(t, srv.ready.Load(), "ready should be false after shutdown")
}

func TestServer_Shutdown_FlushesOTelProviders(t *testing.T) {
	cfg := testConfig()
	cfg.Server.Port = 0
	cfg.Server.DrainWaitSeconds = 0

	exporter := tracetest.NewInMemoryExporter()
	srv, err := NewServer(cfg, testLogger(), exporter)
	require.NoError(t, err)

	// Start in background.
	go func() { _ = srv.Start() }()
	time.Sleep(100 * time.Millisecond)

	// Shutdown should flush both metrics and traces without error.
	err = srv.Shutdown(5 * time.Second)
	require.NoError(t, err, "Shutdown should flush OTel providers without error")
}

func TestServer_AddLivenessCheck(t *testing.T) {
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil)
	require.NoError(t, err)

	called := false
	srv.AddLivenessCheck("test", func(_ context.Context) error {
		called = true

		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)

	assert.True(t, called, "liveness check should have been called")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestServer_AddReadinessCheck(t *testing.T) {
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil)
	require.NoError(t, err)

	// Mark ready so the drain flag doesn't mask the check result.
	srv.SetReady(true)

	called := false
	srv.AddReadinessCheck("test", func(_ context.Context) error {
		called = true

		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)

	assert.True(t, called, "readiness check should have been called")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSlogMiddleware_SkipsHealthEndpoints(t *testing.T) {
	logger := testLogger()
	mw := slogecho.NewWithConfig(logger, slogecho.Config{
		DefaultLevel: slog.LevelInfo,
		Filters: []slogecho.Filter{
			slogecho.IgnorePath("/healthz", "/readyz", "/metrics"),
		},
	})

	e := newTestEcho()
	e.Use(mw)
	e.GET("/healthz", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/readyz", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/metrics", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// These should not cause any logging errors.
	for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "path: %s", path)
	}
}

func newTestEcho() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	return e
}
