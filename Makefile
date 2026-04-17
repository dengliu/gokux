.PHONY: test lint vet clean

# Run library tests
test:
	go test -v -race -coverprofile=coverage.out ./...

# Lint with golangci-lint
lint:
	golangci-lint run ./...

# Vet
vet:
	go vet ./...

# Remove build artifacts
clean:
	rm -rf coverage.out
