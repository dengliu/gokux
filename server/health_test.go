package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestContext(e *echo.Echo, method, path string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()

	return e.NewContext(req, rec), rec
}

func parseBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()

	var body map[string]string
	err := json.Unmarshal(rec.Body.Bytes(), &body)
	require.NoError(t, err, "response body should be valid JSON")

	return body
}

// --- Healthz tests ---

func TestHealthz_NoChecks(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)

	e := echo.New()
	c, rec := newTestContext(e, http.MethodGet, "/healthz")

	err := h.Healthz(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := parseBody(t, rec)
	assert.Equal(t, "alive", body["status"])
}

func TestHealthz_AllPass(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)
	h.AddLivenessCheck("db", func() error { return nil })
	h.AddLivenessCheck("cache", func() error { return nil })

	e := echo.New()
	c, rec := newTestContext(e, http.MethodGet, "/healthz")

	err := h.Healthz(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := parseBody(t, rec)
	assert.Equal(t, "alive", body["status"])
	assert.Equal(t, "ok", body["db"])
	assert.Equal(t, "ok", body["cache"])
}

func TestHealthz_OneFails(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)
	h.AddLivenessCheck("db", func() error { return nil })
	h.AddLivenessCheck("cache", func() error { return errors.New("connection refused") })

	e := echo.New()
	c, rec := newTestContext(e, http.MethodGet, "/healthz")

	err := h.Healthz(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	body := parseBody(t, rec)
	assert.Equal(t, "unhealthy", body["status"])
	assert.Equal(t, "ok", body["db"])
	assert.Equal(t, "connection refused", body["cache"])
}

// --- Readyz tests ---

func TestReadyz_NoChecks_Ready(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)

	e := echo.New()
	c, rec := newTestContext(e, http.MethodGet, "/readyz")

	err := h.Readyz(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := parseBody(t, rec)
	assert.Equal(t, "ready", body["status"])
}

func TestReadyz_NoChecks_Draining(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(false) // simulate shutdown drain

	h := newHealthHandler(ready)

	e := echo.New()
	c, rec := newTestContext(e, http.MethodGet, "/readyz")

	err := h.Readyz(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	body := parseBody(t, rec)
	assert.Equal(t, "not ready", body["status"])
	assert.Equal(t, "shutting down", body["drain"])
}

func TestReadyz_AllPass(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)
	h.AddReadinessCheck("db", func() error { return nil })
	h.AddReadinessCheck("cache", func() error { return nil })

	e := echo.New()
	c, rec := newTestContext(e, http.MethodGet, "/readyz")

	err := h.Readyz(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := parseBody(t, rec)
	assert.Equal(t, "ready", body["status"])
	assert.Equal(t, "ok", body["db"])
	assert.Equal(t, "ok", body["cache"])
}

func TestReadyz_OneFails(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)
	h := newHealthHandler(ready)
	h.AddReadinessCheck("db", func() error { return errors.New("connection lost") })
	h.AddReadinessCheck("cache", func() error { return nil })

	e := echo.New()
	c, rec := newTestContext(e, http.MethodGet, "/readyz")

	err := h.Readyz(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	body := parseBody(t, rec)
	assert.Equal(t, "not ready", body["status"])
	assert.Equal(t, "connection lost", body["db"])
	assert.Equal(t, "ok", body["cache"])
}

func TestReadyz_DrainPlusChecks(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(false) // draining
	h := newHealthHandler(ready)
	h.AddReadinessCheck("db", func() error { return nil }) // check passes, but drain overrides

	e := echo.New()
	c, rec := newTestContext(e, http.MethodGet, "/readyz")

	err := h.Readyz(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	body := parseBody(t, rec)
	assert.Equal(t, "not ready", body["status"])
	assert.Equal(t, "shutting down", body["drain"])
	assert.Equal(t, "ok", body["db"])
}
