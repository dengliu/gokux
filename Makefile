.PHONY: test lint clean tag

# Run library tests
test:
	go test -v -race -coverprofile=coverage.out ./...

# Lint with golangci-lint via Docker (no local install needed)
lint:
	docker run --rm \
		-v $(PWD):/app \
		-v $(HOME)/go/pkg/mod:/root/go/pkg/mod:ro \
		-w /app \
		golangci/golangci-lint:latest \
		golangci-lint run ./...

# Remove build artifacts
clean:
	rm -rf coverage.out

# Tag the current commit with a semver version and push it.
# Runs tests and lint first; pushes local commits before tagging.
# Usage: make tag VERSION=v0.2.0
tag:
	@test -n "$(VERSION)" || (echo "Usage: make tag VERSION=v0.2.0" && exit 1)
	@echo "$(VERSION)" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+' || \
		(echo "VERSION must be semver (e.g., v0.2.0)" && exit 1)
	@git diff --quiet || (echo "error: working tree is dirty — commit or stash first" && exit 1)
	@echo "==> Running tests..."
	$(MAKE) test
	@echo "==> Running lint..."
	go vet ./...
	@echo "==> Pushing local commits..."
	git push origin HEAD
	@echo "==> Tagging $(VERSION)..."
	git tag -a "$(VERSION)" -m "Release $(VERSION)"
	git push origin "$(VERSION)"
