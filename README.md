# gokux

A lightweight Go microservice showcasing best practices for running in Kubernetes.

Inspired by [stefanprodan/podinfo](https://github.com/stefanprodan/podinfo).

## Features

- **Health checks** — Kubernetes liveness (`/healthz`) and readiness (`/readyz`) probes
- **Prometheus metrics** — HTTP request duration, request count, and Go runtime metrics at `/metrics`
- **12-factor config** — Environment-based configuration via [konf](https://github.com/nil-go/konf)
- **Structured logging** — JSON logging with [zap](https://github.com/uber-go/zap)
- **Graceful shutdown** — Clean shutdown on `SIGINT`/`SIGTERM` with request draining
- **Multi-arch images** — `linux/amd64` and `linux/arm64` via Docker buildx and GitHub Actions

## API

| Endpoint      | Method | Description                                    |
|---------------|--------|------------------------------------------------|
| `/healthz`    | GET    | Liveness probe — returns `200` when alive      |
| `/readyz`     | GET    | Readiness probe — returns `200` when ready, `503` during drain |
| `/metrics`    | GET    | Prometheus metrics (request duration + Go runtime) |

## Quick Start

```bash
# Build and run locally
make run

# Or run directly with Go
go run ./cmd/gokux
```

The server starts on port `8080` by default.

## Configuration

All configuration is via environment variables with the `GOKUX_` prefix:

| Variable             | Default  | Description           |
|----------------------|----------|-----------------------|
| `GOKUX_SERVER_PORT`  | `8080`   | HTTP server port      |
| `GOKUX_LOG_LEVEL`    | `info`   | Log level (debug, info, warn, error) |

## Docker

```bash
# Build multi-arch image
make docker-build

# Build and push
make docker-push
```

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
├── cmd/gokux/
│   └── main.go              # Entrypoint, signal handling, graceful shutdown
├── pkg/
│   ├── api/
│   │   ├── server.go        # Echo server, routing, middleware
│   │   ├── health.go        # /healthz and /readyz handlers
│   │   └── metrics.go       # Prometheus middleware and /metrics handler
│   └── config/
│       └── config.go        # konf-based 12-factor configuration
├── .github/workflows/
│   └── release.yml          # Multi-arch Docker build with GitHub Actions
├── Dockerfile               # Multi-stage build (Go builder → distroless)
├── Makefile                 # Build, test, lint, Docker targets
└── README.md
```

## License

MIT
