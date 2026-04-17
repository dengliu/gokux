# gokux

A lightweight Go microservice showcasing best practices for running in Kubernetes.

Inspired by [stefanprodan/podinfo](https://github.com/stefanprodan/podinfo).

## Features

- **Health checks** — Kubernetes liveness (`/healthz`) and readiness (`/readyz`) probes
- **Prometheus metrics** — HTTP request duration, request count, and Go runtime metrics at `/metrics`
- **12-factor config** — Environment-based configuration via [konf](https://github.com/nil-go/konf)
- **Structured logging** — `log/slog` interface with [zap](https://github.com/uber-go/zap) backend via [slog-zap](https://github.com/samber/slog-zap)
- **Graceful shutdown** — Clean shutdown on `SIGINT`/`SIGTERM` with configurable drain wait and shutdown timeout
- **Multi-arch images** — `linux/amd64` and `linux/arm64` via Docker buildx and GitHub Actions

## API

| Endpoint      | Method | Description                                    |
|---------------|--------|------------------------------------------------|
| `/healthz`    | GET    | Liveness probe — returns `200` when alive      |
| `/readyz`     | GET    | Readiness probe — returns `200` when ready, `503` during drain |
| `/metrics`    | GET    | Prometheus metrics (request duration + Go runtime) |

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

### Lifecycle: New → Init → (register routes) → Run

1. **`gokux.New(opts...)`** — creates an App with configuration options
2. **`app.Init()`** — loads config, creates logger, builds the HTTP server. After this, `app.Config`, `app.Logger`, and `app.Server` are available.
3. **Register routes/middleware** — use `app.Server.Echo` directly (supports global, per-group, and per-route middleware)
4. **`app.Run()`** — starts the server, blocks until shutdown signal, performs graceful shutdown

> **Note:** If you skip `Init()`, `Run()` calls it automatically. The explicit `Init()` is only needed when you want to access `app.Logger`, register routes, or add health checks before starting.

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

### Available Options

| Option | Description |
|---|---|
| `WithConfigFiles(files...)` | YAML config files to load (later overrides earlier) |
| `WithEnvPrefix(prefix)` | Environment variable prefix (default: `GOKUX_`) |
| `WithLogger(logger)` | Provide a pre-configured `*slog.Logger` |

### What You Get for Free

- `/healthz` — Kubernetes liveness probe
- `/readyz` — Kubernetes readiness probe (fails during drain)
- `/metrics` — Prometheus metrics (request duration, count, Go runtime)
- Structured logging (zap + slog)
- Graceful shutdown with configurable drain wait and timeout
- Signal handling (SIGINT/SIGTERM)

## Quick Start

```bash
# Build and run locally
make run

# Or run the example directly with Go
go run ./examples/simpleapp -f config.yaml

# Cascading config files (later overrides earlier)
go run ./examples/simpleapp -f config.yaml -f config-dev.yaml
```

The server starts on port `8080` by default.

## Configuration

Configuration is layered with the following precedence (highest wins):

1. **Environment variables** (`GOKUX_*` prefix)
2. **Last `-f` config file**
3. **Earlier `-f` config files**
4. **Built-in defaults**

### Config files (`-f` flag)

Use `-f` to load YAML config files. The flag can be repeated — later files override earlier ones, similar to `helm install -f values.yaml -f values-dev.yaml`:

```bash
# Single file
gokux -f config.yaml

# Cascading (dev overrides base)
gokux -f config_base.yaml -f config_dev.yaml

# No files — pure env vars (defaults still apply)
GOKUX_SERVER_PORT=9090 gokux
```

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

## Docker

```bash
# Build multi-arch image
make docker-build

# Build and push
make docker-push
```

## Graceful Shutdown

When gokux receives `SIGINT` or `SIGTERM`, it performs a two-phase graceful shutdown:

```
SIGTERM received
    │
    ▼
1. ready.Store(false)          ← readiness probe starts failing
    │
    ▼
2. time.Sleep(drain_wait)      ← "drain_wait_seconds" (default 3)
    │                            Purpose: give the load balancer / kube-proxy
    │                            time to notice the failed readiness probe and
    │                            stop routing NEW requests to this pod.
    │                            During this window the server is still running
    │                            and finishing in-flight requests.
    │
    ▼
3. echo.Shutdown(ctx)          ← "shutdown_timeout_seconds" (default 10)
    │                            Purpose: hard deadline for Echo to finish
    │                            processing any remaining in-flight HTTP
    │                            requests. If they don't complete within this
    │                            window, Shutdown returns an error and the
    │                            connections are forcibly closed.
    │
    ▼
4. Process exits
```

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
