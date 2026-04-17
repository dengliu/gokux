# Build stage
FROM golang:1.26.2-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy the library source (build context should be the repo root)
COPY go.mod go.sum ./
COPY gokux.go option.go ./
COPY server/ server/
COPY config/ config/
COPY logging/ logging/

# Copy the example
COPY examples/simpleapp/ examples/simpleapp/

# Build from the example directory
WORKDIR /app/examples/simpleapp
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /simpleapp \
    .

# Runtime stage
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /simpleapp /simpleapp
COPY examples/simpleapp/config.yaml /config.yaml

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/simpleapp", "-f", "/config.yaml"]
