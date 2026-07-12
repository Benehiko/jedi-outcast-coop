BINARY      := jk2coop
MODULE      := github.com/Benehiko/jedi-outcast-coop
INSTALL_DIR := $(HOME)/.local/bin

GO      := go
GOFLAGS := -mod=vendor

# Version metadata injected into the binary at build time. Override on the
# command line for release builds (e.g. `make build VERSION=v0.1.0`).
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

VERSION_PKG := $(MODULE)/cmd
LDFLAGS     := -s -w \
  -X $(VERSION_PKG).version=$(VERSION) \
  -X $(VERSION_PKG).commit=$(COMMIT) \
  -X $(VERSION_PKG).date=$(DATE)

.PHONY: all build install clean lint fmt test hooks

all: build

build:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) .

# Mirror the CI lint job locally: format check (gofumpt + goimports) then lint.
lint:
	golangci-lint fmt --diff
	golangci-lint run --timeout=5m ./...

# Apply formatters in place (gofumpt + goimports).
fmt:
	golangci-lint fmt

test:
	$(GO) test $(GOFLAGS) -race ./...

# Enable the tracked git hooks (pre-commit lint + build) for this clone.
hooks:
	git config core.hooksPath .githooks
	@echo "git hooks enabled (core.hooksPath=.githooks)"

install: build
	mkdir -p $(INSTALL_DIR)
	install -m 755 $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed to $(INSTALL_DIR)/$(BINARY)"

clean:
	rm -f $(BINARY)
