package server

import (
	"context"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	prometheusexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// metricsProvider holds the OTel meter provider and instruments for HTTP metrics.
type metricsProvider struct {
	provider   *sdkmetric.MeterProvider
	registry   *prometheus.Registry
	duration   metric.Float64Histogram
	count      metric.Int64Counter
	panicCount metric.Int64Counter
}

// newMetricsProvider creates an OTel MeterProvider with a dedicated Prometheus
// registry and registers HTTP server request instruments following OTel semantic conventions.
func newMetricsProvider() (*metricsProvider, error) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	exporter, err := prometheusexporter.New(
		prometheusexporter.WithRegisterer(registry),
	)
	if err != nil {
		return nil, err
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	meter := provider.Meter("github.com/dengliu/gokux/server")

	duration, err := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of HTTP server requests."),
	)
	if err != nil {
		return nil, err
	}

	count, err := meter.Int64Counter(
		"http.server.request.count",
		metric.WithUnit("{request}"),
		metric.WithDescription("Total number of HTTP server requests."),
	)
	if err != nil {
		return nil, err
	}

	// Prometheus exporter emits this as gokux_panic_total, labelled by
	// where={handler,task} so ops can alert on panics from either origin.
	panicCount, err := meter.Int64Counter(
		"gokux.panic",
		metric.WithUnit("{panic}"),
		metric.WithDescription("Total panics recovered, labelled by origin (handler or task)."),
	)
	if err != nil {
		return nil, err
	}

	return &metricsProvider{
		provider:   provider,
		registry:   registry,
		duration:   duration,
		count:      count,
		panicCount: panicCount,
	}, nil
}

// Shutdown flushes and stops the meter provider.
func (m *metricsProvider) Shutdown(ctx context.Context) error {
	return m.provider.Shutdown(ctx)
}

// metricsHandler returns an echo.HandlerFunc that serves Prometheus metrics
// from the provider's dedicated registry.
// GET /metrics — returns HTTP request duration and Go runtime metrics
// in Prometheus exposition format via the OTel Prometheus exporter.
func metricsHandler(mp *metricsProvider) echo.HandlerFunc {
	h := promhttp.HandlerFor(mp.registry, promhttp.HandlerOpts{})

	return func(c echo.Context) error {
		h.ServeHTTP(c.Response(), c.Request())

		return nil
	}
}

// metricsMiddleware returns an Echo middleware that records request duration
// and count using OTel instruments with semantic convention attributes.
// Health check and metrics endpoints are skipped to reduce noise.
func metricsMiddleware(mp *metricsProvider) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Path()
			// Skip metrics for health check and metrics endpoints.
			if path == "/healthz" || path == "/readyz" || path == "/metrics" {
				return next(c)
			}

			start := time.Now()
			err := next(c)
			duration := time.Since(start).Seconds()
			status := strconv.Itoa(c.Response().Status)

			attrs := metric.WithAttributes(
				httpRequestMethodAttr(c.Request().Method),
				urlPathAttr(path),
				httpResponseStatusCodeAttr(status),
			)

			mp.duration.Record(c.Request().Context(), duration, attrs)
			mp.count.Add(c.Request().Context(), 1, attrs)

			return err
		}
	}
}

// OTel semantic convention attribute helpers.

func httpRequestMethodAttr(method string) attribute.KeyValue {
	return attribute.String("http.request.method", method)
}

func urlPathAttr(path string) attribute.KeyValue {
	return attribute.String("url.path", path)
}

func httpResponseStatusCodeAttr(status string) attribute.KeyValue {
	return attribute.String("http.response.status_code", status)
}
