package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthz_NoChecks(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)

	e := newTestEcho()
	e.GET("/healthz", h.Healthz)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"alive"`)
}

func TestHealthz_AllChecksPass(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)
	h.AddLivenessCheck("db", func(_ context.Context) error { return nil })
	h.AddLivenessCheck("cache", func(_ context.Context) error { return nil })

	e := newTestEcho()
	e.GET("/healthz", h.Healthz)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"alive"`)
}

func TestHealthz_CheckFails(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)
	h.AddLivenessCheck("db", func(_ context.Context) error { return errors.New("connection refused") })

	e := newTestEcho()
	e.GET("/healthz", h.Healthz)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "connection refused")
}

func TestReadyz_Ready(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)

	e := newTestEcho()
	e.GET("/readyz", h.Readyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"ready"`)
}

func TestReadyz_NotReady(t *testing.T) {
	ready := &atomic.Bool{}
	h := newHealthHandler(ready)

	e := newTestEcho()
	e.GET("/readyz", h.Readyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "shutting down")
}

func TestReadyz_CheckFails(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)
	h.AddReadinessCheck("db", func(_ context.Context) error { return errors.New("not connected") })

	e := newTestEcho()
	e.GET("/readyz", h.Readyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "not connected")
}

func TestReadyz_WithCustomCheckPassing(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)

	called := false
	h.AddReadinessCheck("custom", func(_ context.Context) error {
		called = true
		return nil
	})

	e := newTestEcho()
	e.GET("/readyz", h.Readyz)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.True(t, called, "custom check should have been invoked")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHealthz_ConcurrentAccess(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)

	// Register checks and hit endpoints concurrently to exercise the mutex.
	done := make(chan struct{})

	go func() {
		for i := range 100 {
			name := "check-" + string(rune('A'+i%26))
			h.AddLivenessCheck(name, func(_ context.Context) error { return nil })
		}
		close(done)
	}()

	e := newTestEcho()
	e.GET("/healthz", h.Healthz)

	for range 100 {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Contains(t, []int{http.StatusOK, http.StatusServiceUnavailable}, rec.Code)
	}

	<-done
}

func TestHealthz_CheckReceivesContext(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)

	h.AddLivenessCheck("ctx-check", func(ctx context.Context) error {
		// The context should have a deadline (from checkTimeout).
		if _, ok := ctx.Deadline(); !ok {
			t.Error("expected context to have a deadline")
		}
		return nil
	})

	e := newTestEcho()
	e.GET("/healthz", h.Healthz)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHealthz_SlowCheckTimesOut(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)

	h.AddLivenessCheck("slow", func(ctx context.Context) error {
		// Simulate a slow check that respects the context.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})

	e := newTestEcho()
	e.GET("/healthz", h.Healthz)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	e.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	// Should have timed out within checkTimeout (2s) + some buffer.
	assert.Less(t, elapsed, 5*time.Second, "slow check should have been cancelled by context timeout")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "context deadline exceeded")
}
