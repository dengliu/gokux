.PHONY: test lint clean

GOLANGCI_LINT_VERSION := v2.1.6

# Run library tests
test:
	go test -v -race -coverprofile=coverage.out ./...

# Lint with golangci-lint via Docker (no local install needed)
lint:
	docker run --rm \
		-v $(PWD):/app \
		-v $(HOME)/go/pkg/mod:/root/go/pkg/mod:ro \
		-w /app \
		golangci/golangci-lint:$(GOLANGCI_LINT_VERSION) \
		golangci-lint run ./...

# Remove build artifacts
clean:
	rm -rf coverage.out
