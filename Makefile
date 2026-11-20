BINARY  := commando
PREFIX  ?= $(HOME)/.local
VERSION ?= dev
LDFLAGS := -X github.com/liliumapps/commando/internal/cli.version=$(VERSION)

# Docs tooling (Material for MkDocs in a local virtualenv).
VENV   := .venv
PYTHON ?= python3
MKDOCS := $(VENV)/bin/mkdocs

.PHONY: all build test test-short test-race test-install vet fmt fmt-check lint check install clean run \
        docs-install docs-serve docs-build docs-gifs docs-clean

all: vet test build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./...

# Skip the slow binary-build integration test.
test-short:
	go test -short ./...

test-race:
	go test -race ./...

test-install:
	sh scripts/install_test.sh

vet:
	go vet ./...

fmt:
	gofmt -w .

# Fail if any file is not gofmt-clean (for the check gate).
fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

# Requires golangci-lint (see .golangci.yml). Install:
#   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
lint:
	golangci-lint run ./...

# Full local gate: formatting, vet, lint, race-enabled tests, and installer tests.
check: fmt-check vet lint test-race test-install

install: build
	install -d $(PREFIX)/bin
	install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)
	@echo "Installed to $(PREFIX)/bin/$(BINARY)"
	@echo "Zsh:  add 'eval \"\$$($(BINARY) init zsh)\"' to ~/.zshrc"
	@echo "Bash: add 'eval \"\$$($(BINARY) init bash)\"' to ~/.bashrc"
	@echo "Fish: add '$(BINARY) init fish | source' to ~/.config/fish/config.fish"

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)
	go clean

# --- Documentation (Material for MkDocs) ------------------------------------

# Create the docs virtualenv and install dependencies (idempotent).
docs-install:
	@test -x $(MKDOCS) || $(PYTHON) -m venv $(VENV)
	$(VENV)/bin/pip install -q -r docs/requirements.txt
	@echo "Docs deps installed in $(VENV)"

# Live-reloading preview at http://127.0.0.1:8000
docs-serve: docs-install
	$(MKDOCS) serve

# Build the static site to ./site (strict: broken links/anchors fail the build).
docs-build: docs-install
	$(MKDOCS) build --strict

# Regenerate the docs GIFs from the .tape scripts in an isolated sandbox.
# Requires vhs (brew install vhs), plus go and ffmpeg.
docs-gifs:
	bash docs/recordings/record.sh

# Remove the built site, the docs virtualenv, and the recording sandbox.
docs-clean:
	rm -rf site $(VENV)
	@test -d docs/recordings/.sandbox && chmod -R u+w docs/recordings/.sandbox || true
	rm -rf docs/recordings/.sandbox
