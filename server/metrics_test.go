package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetricsProvider(t *testing.T) {
	mp, err := newMetricsProvider()
	require.NoError(t, err)

	assert.NotNil(t, mp.provider)
	assert.NotNil(t, mp.registry)
	assert.NotNil(t, mp.duration)
	assert.NotNil(t, mp.count)
	assert.NotNil(t, mp.panicCount)
}

// Regression test for #27: after a handler panics, /metrics must expose
// the panic counter in Prometheus format (`gokux_panic_total`) with a
// label `where="handler"` so ops can alert on handler panics.
func TestPanicCounter_VisibleOnMetrics(t *testing.T) {
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil)
	require.NoError(t, err)

	srv.Echo.GET("/boom", func(c echo.Context) error {
		panic("bang")
	})

	// Trigger a handler panic.
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	// Scrape /metrics and assert the panic counter is present and labelled.
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec = httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "gokux_panic_total", "expected gokux_panic_total in /metrics")
	assert.Contains(t, body, `where="handler"`, "expected where=\"handler\" label on panic counter")
}

func TestMetricsProvider_Shutdown(t *testing.T) {
	mp, err := newMetricsProvider()
	require.NoError(t, err)

	err = mp.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestMetricsHandler_ServesPrometheus(t *testing.T) {
	mp, err := newMetricsProvider()
	require.NoError(t, err)

	e := echo.New()
	e.GET("/metrics", metricsHandler(mp))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	// Should contain Go runtime metrics from the dedicated registry.
	assert.Contains(t, rec.Body.String(), "go_goroutines")
}

func TestMetricsMiddleware_RecordsMetrics(t *testing.T) {
	mp, err := newMetricsProvider()
	require.NoError(t, err)

	e := echo.New()
	e.Use(metricsMiddleware(mp))
	e.GET("/api/hello", func(c echo.Context) error {
		return c.String(http.StatusOK, "hello")
	})
	e.GET("/metrics", metricsHandler(mp))

	// Make a request to /api/hello to generate metrics.
	req := httptest.NewRequest(http.MethodGet, "/api/hello", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Now scrape /metrics and verify the OTel metrics are present.
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	// OTel Prometheus exporter converts dots to underscores in metric names.
	assert.Contains(t, body, "http_server_request_duration", "should contain duration histogram")
	assert.Contains(t, body, "http_server_request_count", "should contain request count")
	// Verify semantic convention attributes are present.
	assert.Contains(t, body, "http_request_method")
	assert.Contains(t, body, "url_path")
	assert.Contains(t, body, "http_response_status_code")
}

func TestMetricsMiddleware_SkipsHealthEndpoints(t *testing.T) {
	mp, err := newMetricsProvider()
	require.NoError(t, err)

	e := echo.New()
	e.Use(metricsMiddleware(mp))
	e.GET("/healthz", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/readyz", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/metrics", metricsHandler(mp))

	// Hit health endpoints — these should NOT generate metrics.
	for _, path := range []string{"/healthz", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Scrape /metrics — should NOT contain any http_server_request data
	// because only health endpoints were hit (which are skipped).
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.NotContains(t, body, "http_server_request_duration_seconds_bucket",
		"health endpoints should not generate duration metrics")
}

func TestAttributeHelpers(t *testing.T) {
	method := httpRequestMethodAttr("GET")
	assert.Equal(t, "http.request.method", string(method.Key))
	assert.Equal(t, "GET", method.Value.AsString())

	path := urlPathAttr("/api/hello")
	assert.Equal(t, "url.path", string(path.Key))
	assert.Equal(t, "/api/hello", path.Value.AsString())

	status := httpResponseStatusCodeAttr("200")
	assert.Equal(t, "http.response.status_code", string(status.Key))
	assert.Equal(t, "200", status.Value.AsString())
}
