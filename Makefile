.PHONY: all build install test vet fmt clean help doctor

BINARY_NAME=scout
BUILD_DIR=build
CMD_DIR=cmd/$(BINARY_NAME)

# Version from git tags (Ghost-style): v0.4.0, v0.4.0-3-gabc1234, or dev.
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT=$(shell git rev-parse --short=8 HEAD 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X github.com/ianclemence/scout/pkg/version.Version=$(VERSION)"

GO?=go

all: build

build:
	mkdir -p $(BUILD_DIR)
	$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)

install: build
	install -m 0755 $(BUILD_DIR)/$(BINARY_NAME) $(HOME)/.local/bin/$(BINARY_NAME)
	$(HOME)/.local/bin/$(BINARY_NAME) init 2>/dev/null || true

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -l .

doctor: build
	$(BUILD_DIR)/$(BINARY_NAME) doctor

clean:
	rm -rf $(BUILD_DIR)

help:
	@echo "build    compile $(BINARY_NAME) into ./build (version $(VERSION), $(GIT_COMMIT))"
	@echo "install  build + install to ~/.local/bin + init"
	@echo "test     go test ./..."
	@echo "vet      go vet ./..."
	@echo "fmt      list unformatted files"
	@echo "doctor   build + run diagnostics"
	@echo "clean    remove ./build"
