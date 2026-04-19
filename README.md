# gokux

A lightweight Go library for building Kubernetes-ready services with built-in health checks, metrics, tracing, structured logging, background tasks, and graceful shutdown.

Inspired by [stefanprodan/podinfo](https://github.com/stefanprodan/podinfo).

## Features

- **Health checks** — Kubernetes liveness (`/healthz`) and readiness (`/readyz`) probes
- **OpenTelemetry metrics** — HTTP request duration and count following [OTel semantic conventions](https://opentelemetry.io/docs/specs/semconv/http/http-metrics/), exposed at `/metrics` in Prometheus format
- **12-factor config** — Environment-based configuration via [konf](https://github.com/nil-go/konf)
- **Structured logging** — `log/slog` interface with [zap](https://github.com/uber-go/zap) backend via [slog-zap](https://github.com/samber/slog-zap), HTTP request logging via [slog-echo](https://github.com/samber/slog-echo) with automatic OTel trace/span ID correlation
- **Distributed tracing** — OTel tracing with automatic HTTP spans and W3C context propagation
- **Background tasks** — Managed long-running goroutines with context-based shutdown and cleanup callbacks via `TaskRunner`
- **Panic recovery** — Automatic recovery from panics in HTTP handlers (Echo `Recover` middleware) and background tasks, preventing a single failure from crashing the process
- **Graceful shutdown** — Clean shutdown on `SIGINT`/`SIGTERM` with configurable drain wait and shutdown timeout
- **Multi-arch images** — `linux/amd64` and `linux/arm64` via Docker buildx and GitHub Actions

## API

| Endpoint      | Method | Description                                    |
|---------------|--------|------------------------------------------------|
| `/healthz`    | GET    | Liveness probe — returns `200` when alive      |
| `/readyz`     | GET    | Readiness probe — returns `200` when ready, `503` during drain |
| `/metrics`    | GET    | OpenTelemetry metrics in Prometheus format          |

## Using as a Library

Other developers can import gokux to build their own Kubernetes-ready service:

```go
package main

import (
    "net/http"

    "github.com/dengliu/gokux"
    "github.com/labstack/echo/v4"
)

func main() {
    app := gokux.New(
        gokux.WithConfigFiles("config.yaml"),
        gokux.WithEnvPrefix("MYAPP_"),
    )

    // Init loads config, creates logger, and builds the server.
    if err := app.Init(); err != nil {
        panic(err)
    }

    // Register routes — app.Logger, app.Config, app.Server are now available.
    app.Server.Echo.GET("/api/hello", func(c echo.Context) error {
        app.Logger.Info("handling request", "path", c.Path())
        return c.JSON(http.StatusOK, map[string]string{"message": "hello"})
    })

    // Run starts the server and blocks until SIGINT/SIGTERM.
    if err := app.Run(); err != nil {
        panic(err)
    }
}
```

### Lifecycle: New → Init → (register routes & tasks) → Run

1. **`gokux.New(opts...)`** — creates an App with configuration options
2. **`app.Init()`** — loads config, creates logger, builds the HTTP server and task runner. After this, `app.Config`, `app.Logger`, `app.Server`, and `app.TaskRunner` are available.
3. **Register routes/middleware/tasks** — use `app.Server.Echo` for routes, `app.TaskRunner` for background tasks
4. **`app.Run(ctx)`** — starts background tasks, starts the server, blocks until shutdown signal, performs graceful shutdown (tasks first, then server)

> **Note:** If you skip `Init()`, `Run()` calls it automatically. The explicit `Init()` is only needed when you want to access `app.Logger`, register routes, add health checks, or register tasks before starting.

### Custom Health Checks

Register dependency checks that are evaluated on every `/healthz` or `/readyz` request:

```go
// Readiness check — /readyz returns 503 if database is down
app.AddReadinessCheck("database", func() error {
    return db.Ping()
})

// Liveness check — /healthz returns 503 if critical subsystem is stuck
app.AddLivenessCheck("worker", func() error {
    if worker.IsStuck() {
        return errors.New("worker goroutine is stuck")
    }
    return nil
})
```

Response example when a check fails (`/readyz`):
```json
{"status": "not ready", "database": "connection refused", "cache": "ok"}
```

**Default behavior (no custom checks registered):**
- `/healthz` always returns `200 {"status": "alive"}` — if the process is running and responding to HTTP, it's alive
- `/readyz` returns `200 {"status": "ready"}` during normal operation, and `503 {"status": "not ready", "drain": "shutting down"}` during graceful shutdown
- The built-in drain detection works regardless of custom checks
- Custom checks are **additive** — they add more conditions that must pass, but the baseline behavior works out of the box

### Background Tasks

Use `app.TaskRunner` to run long-running goroutines that are tied to the application lifecycle. Each task receives a context that is cancelled on shutdown.

```go
import "github.com/dengliu/gokux/job"

// Always-running task with per-task cleanup.
// Run receives a context that is cancelled on shutdown.
// Shutdown receives a FRESH context (not cancelled) with the remaining
// shutdown timeout, so context-aware cleanup like closing connections works.
// Add returns an error if called after the runner has started —
// handle it so registration errors surface at boot.
if err := app.TaskRunner.Add(job.Task{
    Name: "pg-to-redis",
    Run: func(ctx context.Context) error {
        for {
            notification, err := pgConn.WaitForNotification(ctx)
            if ctx.Err() != nil {
                return nil // graceful shutdown
            }
            if err != nil {
                return err
            }
            redisClient.Publish(ctx, notification.Channel, notification.Payload)
        }
    },
    Shutdown: func(ctx context.Context) error {
        return pgConn.Close(ctx) // ctx is still valid here
    },
    // Auto-restart on panic or error with exponential backoff.
    // During shutdown, the task is NOT restarted.
    RestartOnFailure: true,
}); err != nil {
    panic(err)
}

// Global shutdown callbacks for cross-cutting concerns (run in LIFO order,
// after all per-task Shutdown callbacks).
app.TaskRunner.OnShutdown("flush-metrics", func(ctx context.Context) error {
    return flushMetrics(ctx)
})
```

#### Bringing your own scheduler

The TaskRunner intentionally does **not** include cron or interval scheduling — use a dedicated library like [gocron](https://github.com/go-co-op/gocron) and register its scheduler as a task:

```go
import "github.com/go-co-op/gocron/v2"

cronScheduler, _ := gocron.NewScheduler()
cronScheduler.NewJob(gocron.CronJob("0 * * * *", false),
    gocron.NewTask(func() { refreshCache() }))

if err := app.TaskRunner.Add(job.Task{
    Name: "cron-scheduler",
    Run: func(ctx context.Context) error {
        cronScheduler.Start()
        <-ctx.Done()
        return cronScheduler.Shutdown()
    },
}); err != nil {
    panic(err)
}
```

#### Task struct

| Field | Type | Description |
|---|---|---|
| `Name` | `string` | Human-readable identifier for logs |
| `Run` | `func(ctx) error` | The work function; ctx is cancelled on shutdown |
| `Shutdown` | `func(ctx) error` | Optional cleanup; receives a fresh (not cancelled) context with the shutdown deadline |
| `RestartOnFailure` | `bool` | When `true`, auto-restarts the task after a panic or error with exponential backoff (1s → 2s → 4s → … → 60s max). Not restarted during shutdown. Default: `false` |

#### TaskRunner API

| Method | Description |
|---|---|
| `Add(task)` | Register a long-running task (call before `Run`) |
| `OnShutdown(name, fn)` | Register a global cleanup callback (LIFO, runs after per-task Shutdown) |
| `Start(ctx)` | Launch all tasks (called automatically by `app.Run`) |
| `Shutdown(timeout)` | Cancel context → wait for Run to return → per-task Shutdown → global OnShutdown |

### Available Options

| Option | Description |
|---|---|
| `WithConfigFiles(files...)` | YAML config files to load (later overrides earlier) |
| `WithEnvPrefix(prefix)` | Environment variable prefix (default: `GOKUX_`) |
| `WithLogger(logger)` | Provide a pre-configured `*slog.Logger` |
| `WithTraceExporter(exporter)` | OTel SpanExporter for distributed tracing (default: noop) |

### What You Get for Free

- `/healthz` — Kubernetes liveness probe
- `/readyz` — Kubernetes readiness probe (fails during drain)
- `/metrics` — OpenTelemetry metrics in Prometheus format (OTel semantic conventions)
- Structured logging (zap + slog)
- Graceful shutdown with configurable drain wait and timeout
- Signal handling (SIGINT/SIGTERM)
- Background task runner with shutdown callbacks
- Panic recovery in both HTTP handlers and background tasks

## Quick Start

```bash
# Run library tests
make test

# Run the example app
cd examples/simpleapp && make run

# Or directly with Go
go run ./examples/simpleapp -f examples/simpleapp/config.yaml

# Cascading config files (later overrides earlier)
go run ./examples/simpleapp -f config.yaml -f config-dev.yaml
```

The example server starts on port `8080` by default.

## Configuration

Configuration is layered with the following precedence (highest wins):

1. **Environment variables** (configurable prefix, default `GOKUX_`)
2. **Last config file** passed to `WithConfigFiles()`
3. **Earlier config files**
4. **Built-in defaults**

### Config files

Pass YAML config files via `WithConfigFiles()`. Later files override earlier ones, similar to `helm install -f values.yaml -f values-dev.yaml`:

```go
// Single file
app := gokux.New(gokux.WithConfigFiles("config.yaml"))

// Cascading (dev overrides base)
app := gokux.New(gokux.WithConfigFiles("config.yaml", "config-dev.yaml"))

// No files — pure env vars (defaults still apply)
app := gokux.New()
```

> **Tip:** The [simpleapp example](examples/simpleapp/main.go) implements a `-f` CLI flag
> that maps to `WithConfigFiles()`, so you can run it as
> `go run ./examples/simpleapp -f config.yaml -f config-dev.yaml`.

Example `config.yaml`:

```yaml
server:
  port: 8080
  shutdown_timeout_seconds: 10  # hard deadline for in-flight requests during shutdown
  drain_wait_seconds: 3         # pause after marking not-ready, before closing listeners

log:
  level: info
```

### Environment variables

All configuration can also be set via environment variables with the `GOKUX_` prefix:

| Variable                              | Default  | Description           |
|---------------------------------------|----------|-----------------------|
| `GOKUX_SERVER_PORT`                   | `8080`   | HTTP server port      |
| `GOKUX_SERVER_SHUTDOWNTIMEOUTSECONDS` | `10`     | Hard deadline in seconds for in-flight requests during shutdown |
| `GOKUX_SERVER_DRAINWAITSECONDS`       | `3`      | Pause in seconds before closing listeners |
| `GOKUX_LOG_LEVEL`                     | `info`   | Log level (debug, info, warn, error) |

> **Note:** The `GOKUX_` prefix is case-sensitive (must be uppercase). Underscores (`_`) in env var
> names act as hierarchy separators (e.g., `SERVER` → the `server` config section), so multi-word
> field names are concatenated without underscores (e.g., `DRAINWAITSECONDS` maps to the struct
> field `DrainWaitSeconds`). Use YAML config files for more readable multi-word key names like
> `drain_wait_seconds`.

## Metrics

Metrics are instrumented with [OpenTelemetry](https://opentelemetry.io/) and exported in Prometheus format via the OTel Prometheus exporter. The `/metrics` endpoint is compatible with existing Prometheus/Grafana/Datadog scrapers.

### Built-in HTTP metrics (OTel semantic conventions)

| Metric | Type | Unit | Description |
|---|---|---|---|
| `http.server.request.duration` | Histogram | `s` | Duration of HTTP server requests |
| `http.server.request.count` | Counter | `{request}` | Total number of HTTP server requests |

### Attributes

| Attribute | Description | Example |
|---|---|---|
| `http.request.method` | HTTP method | `GET` |
| `url.path` | Request path | `/api/hello` |
| `http.response.status_code` | Response status code | `200` |

Health check (`/healthz`, `/readyz`) and metrics (`/metrics`) endpoints are excluded from instrumentation to reduce noise.

### Custom Metrics

Use `app.Meter(name)` to create application-specific instruments that automatically appear on `/metrics`:

```go
meter := app.Meter("myapp/orders")

orderCount, _ := meter.Int64Counter(
    "orders.created.count",
    metric.WithDescription("Total orders created"),
)

orderDuration, _ := meter.Float64Histogram(
    "orders.processing.duration",
    metric.WithUnit("s"),
    metric.WithDescription("Order processing duration"),
)

// Use in handlers:
app.Server.Echo.POST("/orders", func(c echo.Context) error {
    start := time.Now()
    // ... process order ...
    orderCount.Add(c.Request().Context(), 1)
    orderDuration.Record(c.Request().Context(), time.Since(start).Seconds())
    return c.JSON(http.StatusCreated, order)
})
```

### Architecture

Each server instance creates a dedicated `prometheus.Registry` (no global state), an OTel `MeterProvider` with a Prometheus exporter, and an OTel `TracerProvider` for distributed tracing. This means:
- Multiple servers in tests don't conflict
- The meter provider has explicit lifecycle (created in `NewServer`, can be shut down cleanly)
- Consumers can extend by creating additional OTel instruments on the same meter provider

## Tracing

Distributed tracing is built on [OpenTelemetry](https://opentelemetry.io/). By default, tracing uses a noop provider (zero overhead). Enable it by providing a `SpanExporter`:

```go
import "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"

exporter, _ := stdouttrace.New()  // prints spans to stdout (dev)
app := gokux.New(
    gokux.WithTraceExporter(exporter),
)
```

### Automatic HTTP spans

Every incoming request automatically gets a span via the `otelecho` middleware with OTel semantic convention attributes:

```
GET /api/hello
├── http.request.method: GET
├── url.path: /api/hello
├── http.response.status_code: 200
└── http.route: /api/hello
```

### Custom spans

Use `app.Tracer(name)` to create application-specific spans:

```go
tracer := app.Tracer("myapp/orders")

app.Server.Echo.POST("/orders", func(c echo.Context) error {
    ctx, span := tracer.Start(c.Request().Context(), "process-order")
    defer span.End()
    // ... process order ...
    return c.JSON(http.StatusCreated, order)
})
```

### Production setup (OTLP)

For production, use the OTLP exporter to send traces to an OTel Collector:

```go
import "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"

exporter, _ := otlptracegrpc.New(ctx,
    otlptracegrpc.WithEndpoint("otel-collector:4317"),
    otlptracegrpc.WithInsecure(),
)
app := gokux.New(
    gokux.WithTraceExporter(exporter),
)
```

### Context propagation

W3C TraceContext and Baggage propagation is configured automatically. Trace IDs are extracted from incoming `traceparent` headers and injected into outgoing requests, enabling end-to-end distributed tracing across services.

## Docker

The example app includes a multi-arch Dockerfile:

```bash
cd examples/simpleapp

# Build multi-arch image
make docker-build

# Build and push
make docker-push
```

## Graceful Shutdown

When gokux receives `SIGINT` or `SIGTERM`, it performs a graceful shutdown:

```
SIGTERM received
    │
    ▼
1. TaskRunner.Shutdown()       ← cancel task contexts, run OnShutdown
    │                            callbacks (LIFO), wait for tasks to finish
    │
    ▼
2. ready.Store(false)          ← readiness probe starts failing
    │
    ▼
3. time.Sleep(drain_wait)      ← "drain_wait_seconds" (default 3)
    │                            Purpose: give the load balancer / kube-proxy
    │                            time to notice the failed readiness probe and
    │                            stop routing NEW requests to this pod.
    │                            During this window the server is still running
    │                            and finishing in-flight requests.
    │
    ▼
4. echo.Shutdown(ctx)          ← "shutdown_timeout_seconds" (default 10)
    │                            Purpose: hard deadline for Echo to finish
    │                            processing any remaining in-flight HTTP
    │                            requests. If they don't complete within this
    │                            window, Shutdown returns an error and the
    │                            connections are forcibly closed.
    │
    ▼
5. Process exits
```

> **Why tasks shut down first:** Background tasks may depend on the HTTP server still being available (e.g., sending final metrics, deregistering from a service registry). Shutting them down before the server ensures they can complete their cleanup.

### `drain_wait_seconds` vs `shutdown_timeout_seconds`

| | `drain_wait_seconds` | `shutdown_timeout_seconds` |
|---|---|---|
| **What it controls** | Time to let LB/kube-proxy stop sending new traffic | Time to let in-flight requests finish |
| **Server accepting requests?** | Yes | No (new connections refused) |
| **Default value** | 3s | 10s |
| **Depends on** | LB health check interval, readiness probe period | Longest expected request duration |

### Relationship with Kubernetes `terminationGracePeriodSeconds`

The Kubernetes `terminationGracePeriodSeconds` (default 30s) is the outer envelope — it **must** be greater than or equal to `drain_wait_seconds + shutdown_timeout_seconds`. Otherwise the kubelet will `SIGKILL` the process before graceful shutdown completes.

For example, with the defaults (`drain_wait_seconds=3`, `shutdown_timeout_seconds=10`), the total graceful shutdown takes at most 13 seconds, which fits comfortably within the default 30-second `terminationGracePeriodSeconds`.

## Kubernetes

Example probe configuration:

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

## Project Structure

```
gokux/
├── gokux.go                 # App struct, New(), Run() — reusable entrypoint
├── option.go                # Functional options (WithConfigFiles, WithRoutes, etc.)
├── server/                  # Echo server, health checks, metrics
│   ├── server.go
│   ├── health.go
│   └── metrics.go
├── job/                     # Background task runner with graceful shutdown
│   └── runner.go
├── config/                  # konf-based 12-factor configuration
│   └── config.go
├── logging/                 # Structured logger (zap + slog-zap)
│   └── logging.go
├── examples/
│   └── simpleapp/
│       └── main.go          # Reference example using gokux.New()
├── .github/workflows/
│   └── release.yml          # Multi-arch Docker build with GitHub Actions
├── Dockerfile               # Multi-stage build (Go builder → distroless)
├── Makefile                 # Build, test, lint, Docker targets
└── README.md
```

## License

MIT
