# Build stage
FROM golang:1.25.1-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /app

# Cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

# Build the binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /gokux \
    ./cmd/gokux

# Runtime stage
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /gokux /gokux

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/gokux"]
