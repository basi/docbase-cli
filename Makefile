BINARY_NAME=docbase
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GOPATH ?= $(shell go env GOPATH)
LDFLAGS=-ldflags "-X github.com/basi/docbase-cli/cmd/root.Version=$(VERSION) -X github.com/basi/docbase-cli/cmd/root.BuildTime=$(BUILD_TIME)"

.PHONY: all build clean install test lint fmt vet

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
	@echo "Running linter..."
	@golint ./...

fmt:
	@echo "Running gofmt..."
	@gofmt -s -w .

vet:
	@echo "Running go vet..."
	@go vet ./...

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

# Homebrew formula target
.PHONY: homebrew-formula

homebrew-formula:
	@echo "Generating Homebrew formula..."
	@mkdir -p dist/homebrew
	@echo "class Docbase < Formula" > dist/homebrew/$(BINARY_NAME).rb
	@echo "  desc \"Command-line interface for DocBase\"" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "  homepage \"https://github.com/basi/docbase-cli\"" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "  url \"https://github.com/basi/docbase-cli/archive/v$(VERSION).tar.gz\"" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "  sha256 \"REPLACE_WITH_ACTUAL_SHA256\"" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "  license \"MIT\"" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "  head \"https://github.com/basi/docbase-cli.git\"" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "  depends_on \"go\" => :build" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "  def install" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "    system \"go\", \"build\", *std_go_args, \"-ldflags\", \"-X github.com/basi/docbase-cli/cmd/root.Version=#{version}\"" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "  end" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "  test do" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "    assert_match \"DocBase CLI\", shell_output(\"\#{bin}/docbase --help\")" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "  end" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "end" >> dist/homebrew/$(BINARY_NAME).rb
	@echo "Homebrew formula generated at dist/homebrew/$(BINARY_NAME).rb"
	@echo "Note: You need to replace REPLACE_WITH_ACTUAL_SHA256 with the actual SHA256 of the tarball."