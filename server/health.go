package server

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
)

// checkTimeout is the maximum time a single health check may run before
// its context is cancelled. This prevents slow probes from stacking
// goroutines under Kubernetes' aggressive probe period.
const checkTimeout = 2 * time.Second

// HealthCheck is a function that reports the health of a dependency.
// The context carries a deadline derived from the probe handler; callers
// should pass it to downstream operations (e.g. db.PingContext) so that
// slow checks are cancelled rather than hanging.
// Return nil if healthy, or an error describing the problem.
type HealthCheck func(ctx context.Context) error

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

// runChecks executes each check with a bounded per-check context and
// returns the results map and an allOK flag.
func runChecks(ctx context.Context, checks map[string]HealthCheck) (map[string]string, bool) {
	result := make(map[string]string, len(checks))
	allOK := true

	for name, check := range checks {
		checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		if err := check(checkCtx); err != nil {
			result[name] = err.Error()
			allOK = false
		} else {
			result[name] = "ok"
		}
		cancel()
	}

	return result, allOK
}

// Healthz is the Kubernetes liveness probe handler.
// GET /healthz — returns 200 when the process is alive and all liveness checks pass.
// With no registered liveness checks, it always returns 200 {"status": "alive"}.
func (h *healthHandler) Healthz(c echo.Context) error {
	h.mu.RLock()
	checks := make(map[string]HealthCheck, len(h.livenessChecks))
	for k, v := range h.livenessChecks {
		checks[k] = v
	}
	h.mu.RUnlock()

	result, allOK := runChecks(c.Request().Context(), checks)

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
// With no registered readiness checks, the built-in drain detection still
// works: returns 200 during normal operation, 503 during graceful shutdown.
// Custom checks are additive — they add more conditions that must pass.
func (h *healthHandler) Readyz(c echo.Context) error {
	h.mu.RLock()
	checks := make(map[string]HealthCheck, len(h.readinessChecks))
	for k, v := range h.readinessChecks {
		checks[k] = v
	}
	h.mu.RUnlock()

	result, allOK := runChecks(c.Request().Context(), checks)

	// Check the drain flag.
	if !h.ready.Load() {
		result["drain"] = "shutting down"
		allOK = false
	}

	if allOK {
		result["status"] = "ready"

		return c.JSON(http.StatusOK, result)
	}

	result["status"] = "not ready"

	return c.JSON(http.StatusServiceUnavailable, result)
}
