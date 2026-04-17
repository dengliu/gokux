.PHONY: test lint clean

# Run library tests
test:
	go test -v -race -coverprofile=coverage.out ./...

# Lint with golangci-lint (includes go vet)
lint:
	golangci-lint run ./...

# Remove build artifacts
clean:
	rm -rf coverage.out
