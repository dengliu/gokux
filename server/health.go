package server

import (
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/labstack/echo/v4"
)

// HealthCheck is a function that reports the health of a dependency.
// Return nil if healthy, or an error describing the problem.
type HealthCheck func() error

// healthHandler holds the readiness state and registered health checks.
type healthHandler struct {
	ready           *atomic.Bool
	mu              sync.RWMutex
	livenessChecks  map[string]HealthCheck
	readinessChecks map[string]HealthCheck
}

// newHealthHandler creates a new healthHandler with the ready flag.
func newHealthHandler(ready *atomic.Bool) *healthHandler {
	return &healthHandler{
		ready:           ready,
		livenessChecks:  make(map[string]HealthCheck),
		readinessChecks: make(map[string]HealthCheck),
	}
}

// AddLivenessCheck registers a named liveness check.
// If any liveness check fails, /healthz returns 503.
func (h *healthHandler) AddLivenessCheck(name string, check HealthCheck) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.livenessChecks[name] = check
}

// AddReadinessCheck registers a named readiness check.
// If any readiness check fails (or the server is draining), /readyz returns 503.
func (h *healthHandler) AddReadinessCheck(name string, check HealthCheck) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.readinessChecks[name] = check
}

// Healthz is the Kubernetes liveness probe handler.
// GET /healthz — returns 200 when the process is alive and all liveness checks pass.
func (h *healthHandler) Healthz(c echo.Context) error {
	h.mu.RLock()
	checks := make(map[string]HealthCheck, len(h.livenessChecks))
	for k, v := range h.livenessChecks {
		checks[k] = v
	}
	h.mu.RUnlock()

	result := make(map[string]string, len(checks)+1)
	allOK := true

	for name, check := range checks {
		if err := check(); err != nil {
			result[name] = err.Error()
			allOK = false
		} else {
			result[name] = "ok"
		}
	}

	if allOK {
		result["status"] = "alive"

		return c.JSON(http.StatusOK, result)
	}

	result["status"] = "unhealthy"

	return c.JSON(http.StatusServiceUnavailable, result)
}

// Readyz is the Kubernetes readiness probe handler.
// GET /readyz — returns 200 when the service is ready to accept traffic,
// or 503 during shutdown draining or when any readiness check fails.
func (h *healthHandler) Readyz(c echo.Context) error {
	h.mu.RLock()
	checks := make(map[string]HealthCheck, len(h.readinessChecks))
	for k, v := range h.readinessChecks {
		checks[k] = v
	}
	h.mu.RUnlock()

	result := make(map[string]string, len(checks)+1)
	allOK := true

	// Check the drain flag first.
	if !h.ready.Load() {
		result["drain"] = "shutting down"
		allOK = false
	}

	for name, check := range checks {
		if err := check(); err != nil {
			result[name] = err.Error()
			allOK = false
		} else {
			result[name] = "ok"
		}
	}

	if allOK {
		result["status"] = "ready"

		return c.JSON(http.StatusOK, result)
	}

	result["status"] = "not ready"

	return c.JSON(http.StatusServiceUnavailable, result)
}
