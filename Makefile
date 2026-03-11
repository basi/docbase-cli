BINARY_NAME=docbase
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GOPATH ?= $(shell go env GOPATH)
LDFLAGS=-ldflags "-X github.com/basi/docbase-cli/cmd/root.Version=$(VERSION) -X github.com/basi/docbase-cli/cmd/root.BuildTime=$(BUILD_TIME)"

.PHONY: all build clean install test lint fmt lint-install

all: clean build

build:
	@echo "Building $(BINARY_NAME)..."
	@go build $(LDFLAGS) -o $(BINARY_NAME) .

clean:
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -rf dist/

install: build
	@echo "Installing $(BINARY_NAME)..."
	@mv $(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)

test:
	@echo "Running tests..."
	@go test -v ./...

lint:
	@echo "Running golangci-lint..."
	@golangci-lint run ./...

lint-install:
	@echo "Installing golangci-lint..."
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

fmt:
	@echo "Running golangci-lint fmt..."
	@golangci-lint fmt ./...

# Cross-compilation targets
.PHONY: build-all build-darwin build-linux build-windows

build-all: build-darwin build-linux build-windows

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p dist/darwin
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/darwin/$(BINARY_NAME) .
	@GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/darwin/$(BINARY_NAME)-arm64 .

build-linux:
	@echo "Building for Linux..."
	@mkdir -p dist/linux
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/linux/$(BINARY_NAME) .
	@GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/linux/$(BINARY_NAME)-arm64 .

build-windows:
	@echo "Building for Windows..."
	@mkdir -p dist/windows
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/windows/$(BINARY_NAME).exe .

