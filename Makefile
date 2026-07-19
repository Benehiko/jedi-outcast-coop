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

.PHONY: all build install clean lint fmt test e2e hooks generate verify-embed

all: build

build:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) .

# Regenerate the embedded OpenJK source archive, patches, and coop-ui assets
# from the pinned submodule. Run after bumping the openjk submodule.
generate:
	$(GO) generate ./internal/embed

# CI guard: regenerating the embed must produce no diff. A submodule bump (or a
# patches/ or assets/coop-ui/ edit) that forgets `make generate` fails here, so
# the baked-in source can never silently drift from the pin.
#
# `git diff` alone misses NEW mirror files: a fresh patch (patches/00NN-*.patch)
# regenerates into internal/embed/patches/ as an UNTRACKED file, which `git diff`
# does not report — so a "forgot to commit the mirror" slips through green. Check
# both tracked changes AND untracked files under internal/embed.
verify-embed: generate
	@untracked="$$(git ls-files --others --exclude-standard -- internal/embed)"; \
	if ! git diff --quiet -- internal/embed || [ -n "$$untracked" ]; then \
	  echo "internal/embed is out of sync with the submodule/patches/assets." >&2; \
	  echo "Run 'make generate' and commit the result:" >&2; \
	  git --no-pager diff --stat -- internal/embed >&2; \
	  if [ -n "$$untracked" ]; then \
	    echo "untracked (new) embed files not committed:" >&2; \
	    printf '  %s\n' $$untracked >&2; \
	  fi; \
	  exit 1; \
	fi
	@echo "internal/embed is in sync"

# Mirror the CI lint job locally: format check (gofumpt + goimports) then lint.
# `fmt --diff` exits 0 even when it prints a diff, so fail on a non-empty diff.
lint:
	@out="$$(golangci-lint fmt --diff)"; \
	if [ -n "$$out" ]; then \
	  echo "code is not formatted; run 'make fmt':" >&2; \
	  printf '%s\n' "$$out" >&2; \
	  exit 1; \
	fi
	golangci-lint run --timeout=5m ./...

# Apply formatters in place (gofumpt + goimports).
fmt:
	golangci-lint fmt

test:
	$(GO) test $(GOFLAGS) -race ./...

# End-to-end tests that drive the built binary against the real repo (needs the
# OpenJK submodule checked out and git on PATH). Builds jk2coop first and passes
# it to the tests via JK2COOP_BIN so they don't each rebuild it.
e2e: build
	JK2COOP_BIN=$(CURDIR)/$(BINARY) $(GO) test $(GOFLAGS) -tags e2e -v ./e2e/...

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
