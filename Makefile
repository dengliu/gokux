.PHONY: build run test test-all lint docker-build docker-push clean

# Variables
APP_NAME    := simpleapp
BUILD_DIR   := bin
IMAGE       := ghcr.io/dengliu/gokux
TAG         ?= latest

# Build the example binary
build:
	cd examples/simpleapp && CGO_ENABLED=0 go build -ldflags="-s -w" -o ../../$(BUILD_DIR)/$(APP_NAME) .

# Run the example locally with default config
run: build
	$(BUILD_DIR)/$(APP_NAME) -f examples/simpleapp/config.yaml

# Run library tests
test:
	go test -v -race -coverprofile=coverage.out ./...

# Run all tests (library + example)
test-all: test
	cd examples/simpleapp && go build ./...

# Lint with golangci-lint
lint:
	golangci-lint run ./...

# Build multi-arch Docker image locally
docker-build:
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-f examples/simpleapp/Dockerfile \
		-t $(IMAGE):$(TAG) \
		.

# Build and push multi-arch Docker image
docker-push:
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-f examples/simpleapp/Dockerfile \
		-t $(IMAGE):$(TAG) \
		--push \
		.

# Remove build artifacts
clean:
	rm -rf $(BUILD_DIR) coverage.out
