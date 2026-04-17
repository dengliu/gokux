.PHONY: build run docker-build docker-push clean

APP_NAME := simpleapp
IMAGE    := ghcr.io/dengliu/gokux
TAG      ?= latest

# Build the binary
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(APP_NAME) .

# Run locally with default config
run: build
	./$(APP_NAME) -f config.yaml

# Build multi-arch Docker image (run from repo root)
docker-build:
	cd ../.. && docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-f examples/simpleapp/Dockerfile \
		-t $(IMAGE):$(TAG) \
		.

# Build and push multi-arch Docker image
docker-push:
	cd ../.. && docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-f examples/simpleapp/Dockerfile \
		-t $(IMAGE):$(TAG) \
		--push \
		.

# Remove build artifacts
clean:
	rm -f $(APP_NAME)
