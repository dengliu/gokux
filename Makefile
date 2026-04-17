.PHONY: build run test lint docker-build docker-push clean

# Variables
APP_NAME    := gokux
CMD_PATH    := ./cmd/gokux
BUILD_DIR   := bin
IMAGE       := ghcr.io/dengliu/gokux
TAG         ?= latest

# Build the binary
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME) $(CMD_PATH)

# Run locally with default config
run: build
	$(BUILD_DIR)/$(APP_NAME) -f config.yaml

# Run tests with race detection
test:
	go test -v -race -coverprofile=coverage.out ./...

# Lint with golangci-lint
lint:
	golangci-lint run ./...

# Build multi-arch Docker image locally
docker-build:
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t $(IMAGE):$(TAG) \
		.

# Build and push multi-arch Docker image
docker-push:
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t $(IMAGE):$(TAG) \
		--push \
		.

# Remove build artifacts
clean:
	rm -rf $(BUILD_DIR) coverage.out
