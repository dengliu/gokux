package server

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestDuration, httpRequestsTotal)
}

// MetricsHandler returns an echo.HandlerFunc that serves Prometheus metrics.
// GET /metrics — returns HTTP request duration and Go runtime metrics.
func MetricsHandler() echo.HandlerFunc {
	h := promhttp.Handler()

	return func(c echo.Context) error {
		h.ServeHTTP(c.Response(), c.Request())

		return nil
	}
}

// MetricsMiddleware records request duration and count for every request,
// skipping health check endpoints to avoid noise.
func MetricsMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Path()
			// Skip metrics for health check endpoints.
			if path == "/healthz" || path == "/readyz" || path == "/metrics" {
				return next(c)
			}

			start := time.Now()
			err := next(c)
			duration := time.Since(start).Seconds()
			status := strconv.Itoa(c.Response().Status)

			httpRequestDuration.WithLabelValues(c.Request().Method, path, status).Observe(duration)
			httpRequestsTotal.WithLabelValues(c.Request().Method, path, status).Inc()

			return err
		}
	}
}
