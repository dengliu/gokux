package server

import (
	"net/http"
	"sync/atomic"

	"github.com/labstack/echo/v4"
)

// healthHandler holds the readiness state for health check endpoints.
type healthHandler struct {
	ready *atomic.Bool
}

// newHealthHandler creates a new healthHandler with the ready flag.
func newHealthHandler(ready *atomic.Bool) *healthHandler {
	return &healthHandler{ready: ready}
}

// Healthz is the Kubernetes liveness probe handler.
// GET /healthz — always returns 200 when the process is alive.
func (h *healthHandler) Healthz(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "alive"})
}

// Readyz is the Kubernetes readiness probe handler.
// GET /readyz — returns 200 when the service is ready to accept traffic,
// or 503 during shutdown draining.
func (h *healthHandler) Readyz(c echo.Context) error {
	if h.ready.Load() {
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	}

	return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "not ready"})
}
