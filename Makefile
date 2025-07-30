.PHONY: build test clean install fmt vet lint

# Variables
BINARY_NAME=dbxignore
VERSION?=dev
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.Date=${BUILD_DATE}"

# Build the binary
build:
	go build -trimpath ${LDFLAGS} -o ${BINARY_NAME} cmd/dbxignore/main.go

# Run tests
test:
	go test -race ./...

# Run tests with coverage
test-coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	rm -f ${BINARY_NAME}
	rm -f coverage.out coverage.html
	rm -rf dist/

# Install binary to system
install: build
	cp ${BINARY_NAME} /usr/local/bin/

# Format code
fmt:
	go fmt ./...

# Run go vet
vet:
	go vet ./...

# Run golangci-lint
lint:
	golangci-lint run --fast

# Run all checks
check: fmt vet lint test

# Build for all platforms
build-all:
	GOOS=darwin GOARCH=amd64 go build -trimpath ${LDFLAGS} -o dist/${BINARY_NAME}-darwin-amd64 cmd/dbxignore/main.go
	GOOS=darwin GOARCH=arm64 go build -trimpath ${LDFLAGS} -o dist/${BINARY_NAME}-darwin-arm64 cmd/dbxignore/main.go
	GOOS=linux GOARCH=amd64 go build -trimpath ${LDFLAGS} -o dist/${BINARY_NAME}-linux-amd64 cmd/dbxignore/main.go
	GOOS=linux GOARCH=arm64 go build -trimpath ${LDFLAGS} -o dist/${BINARY_NAME}-linux-arm64 cmd/dbxignore/main.go

# Development build with race detector
dev:
	go build -race -o ${BINARY_NAME} cmd/dbxignore/main.go

# Run the daemon in development mode
run: dev
	./${BINARY_NAME} serve --root ./test-dir --verbose --dry-run