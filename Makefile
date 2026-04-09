# =============================================================================
# LedgerAlps — Developer Makefile
# Works on Linux, macOS, and Windows Git Bash.
# =============================================================================

.DEFAULT_GOAL := help
.PHONY: help build build-server build-cli build-launcher build-windows \
        build-frontend build-installer run test test-coverage lint fmt vet tidy \
        clean docker-up docker-down docker-logs release-snapshot release-dry \
        release frontend-install frontend-build install

# --------------------------------------------------------------------------- #
# Build metadata — injected at link time                                      #
# --------------------------------------------------------------------------- #
BINARY_SERVER   := ledgeralps-server
BINARY_CLI      := ledgeralps-cli
BINARY_LAUNCHER := ledgeralps
DIST_DIR        := dist
MODULE          := github.com/kmdn-ch/ledgeralps
INSTALLER_VER   ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "dev")

VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo "unknown")

LDFLAGS := -s -w \
  -X $(MODULE)/version.version=$(VERSION) \
  -X $(MODULE)/version.commit=$(COMMIT) \
  -X $(MODULE)/version.date=$(DATE) \
  -X $(MODULE)/version.builtBy=make

GO_BUILD := go build -trimpath -ldflags "$(LDFLAGS)"

# --------------------------------------------------------------------------- #
# Help                                                                        #
# --------------------------------------------------------------------------- #
help: ## Show this help
	@printf '\033[1mLedgerAlps $(VERSION)\033[0m — available targets:\n\n'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | sort \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-24s\033[0m %s\n", $$1, $$2}'
	@printf '\n'

# --------------------------------------------------------------------------- #
# Build — native (current OS)                                                 #
# --------------------------------------------------------------------------- #
build: build-server build-cli ## Build server + CLI for the current OS

build-server: ## Build the API server binary (embeds frontend if built)
	@mkdir -p $(DIST_DIR) internal/frontend/dist
	@if [ -d frontend/dist ]; then cp -r frontend/dist/. internal/frontend/dist/; fi
	$(GO_BUILD) -o $(DIST_DIR)/$(BINARY_SERVER) ./cmd/server
	@echo "  built  $(DIST_DIR)/$(BINARY_SERVER)  [$(VERSION) @ $(COMMIT)]"

build-cli: ## Build the admin CLI binary
	@mkdir -p $(DIST_DIR)
	$(GO_BUILD) -o $(DIST_DIR)/$(BINARY_CLI) ./cmd/cli
	@echo "  built  $(DIST_DIR)/$(BINARY_CLI)  [$(VERSION) @ $(COMMIT)]"

# --------------------------------------------------------------------------- #
# Build — Windows installer pipeline                                          #
# --------------------------------------------------------------------------- #
build-launcher: ## Build Windows launcher (ledgeralps.exe, no console window)
	@mkdir -p $(DIST_DIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
	  go build -trimpath -ldflags "$(LDFLAGS) -H=windowsgui" \
	  -o $(DIST_DIR)/$(BINARY_LAUNCHER).exe ./cmd/launcher
	@echo "  built  $(DIST_DIR)/$(BINARY_LAUNCHER).exe  [$(VERSION) @ $(COMMIT)]"

build-windows: ## Build server + launcher for Windows (amd64), both windowsgui (no console)
	@mkdir -p $(DIST_DIR) internal/frontend/dist
	@if [ -d frontend/dist ]; then cp -r frontend/dist/. internal/frontend/dist/; fi
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
	  go build -trimpath -ldflags "$(LDFLAGS) -H=windowsgui" \
	  -o $(DIST_DIR)/$(BINARY_SERVER).exe ./cmd/server
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
	  go build -trimpath -ldflags "$(LDFLAGS) -H=windowsgui" \
	  -o $(DIST_DIR)/$(BINARY_LAUNCHER).exe ./cmd/launcher
	@echo "  built  $(DIST_DIR)/$(BINARY_SERVER).exe + $(DIST_DIR)/$(BINARY_LAUNCHER).exe"

build-frontend: ## Build frontend for production (outputs to frontend/dist/)
	cd frontend && npm ci && npm run build
	@echo "  built  frontend/dist/"

build-installer: build-windows build-frontend ## Build the full Windows installer (.exe) via Inno Setup
	@mkdir -p installer/Output
	ISCC installer/ledgeralps.iss
	@echo "  installer: installer/Output/LedgerAlps_Setup_$(INSTALLER_VER)_windows_amd64.exe"

release: build-installer ## Full release: binaries + frontend + installer
	@echo "Release $(INSTALLER_VER) ready."

# --------------------------------------------------------------------------- #
# Run                                                                         #
# --------------------------------------------------------------------------- #
run: ## Run the API server locally (JWT_SECRET must be set)
	@if [ -z "$$JWT_SECRET" ]; then \
	  echo "ERROR: JWT_SECRET is not set."; \
	  echo "Generate one with:  export JWT_SECRET=$$(openssl rand -hex 32)"; \
	  exit 1; \
	fi
	go run ./cmd/server

# --------------------------------------------------------------------------- #
# Test                                                                        #
# --------------------------------------------------------------------------- #
test: ## Run all tests with race detector
	go test -race ./...

test-coverage: ## Run tests and open HTML coverage report
	go test -race ./... -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out

# --------------------------------------------------------------------------- #
# Code quality                                                                #
# --------------------------------------------------------------------------- #
lint: ## Run golangci-lint (install from https://golangci-lint.run)
	@command -v golangci-lint >/dev/null 2>&1 \
	  || { echo "ERROR: golangci-lint not found. Install: https://golangci-lint.run/usage/install/"; exit 1; }
	golangci-lint run ./...

fmt: ## Format all Go files with gofmt
	gofmt -w .

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy module dependencies
	go mod tidy

# --------------------------------------------------------------------------- #
# Clean                                                                       #
# --------------------------------------------------------------------------- #
clean: ## Remove build artifacts
	rm -rf $(DIST_DIR)/ coverage.out

# --------------------------------------------------------------------------- #
# Docker                                                                      #
# --------------------------------------------------------------------------- #
docker-up: ## Start all services (builds images)
	cp -n .env.example .env 2>/dev/null || true
	docker compose up -d --build
	@echo "  server:   http://localhost:8000"
	@echo "  frontend: http://localhost:5173"

docker-down: ## Stop all docker compose services
	docker compose down

docker-logs: ## Follow docker compose logs
	docker compose logs -f

# --------------------------------------------------------------------------- #
# GoReleaser                                                                  #
# --------------------------------------------------------------------------- #
release-snapshot: ## Build a local snapshot with goreleaser (no publish)
	@command -v goreleaser >/dev/null 2>&1 \
	  || { echo "ERROR: goreleaser not found. Install: https://goreleaser.com/install/"; exit 1; }
	goreleaser release --snapshot --clean

release-dry: ## Dry-run goreleaser (validates, builds, no upload)
	@command -v goreleaser >/dev/null 2>&1 \
	  || { echo "ERROR: goreleaser not found. Install: https://goreleaser.com/install/"; exit 1; }
	goreleaser release --skip=publish --clean

# --------------------------------------------------------------------------- #
# Frontend (alias targets)                                                    #
# --------------------------------------------------------------------------- #
frontend-install: ## Install frontend npm dependencies
	cd frontend && npm install

frontend-build: build-frontend ## Alias for build-frontend

# --------------------------------------------------------------------------- #
# Install (Linux / macOS)                                                     #
# --------------------------------------------------------------------------- #
install: build ## Install both binaries to /usr/local/bin
	@case "$$(uname -s)" in \
	  Linux|Darwin) \
	    install -m 0755 $(DIST_DIR)/$(BINARY_SERVER) /usr/local/bin/$(BINARY_SERVER); \
	    install -m 0755 $(DIST_DIR)/$(BINARY_CLI) /usr/local/bin/$(BINARY_CLI); \
	    echo "Installed $(BINARY_SERVER) and $(BINARY_CLI) to /usr/local/bin/";; \
	  *) echo "ERROR: 'make install' is Linux/macOS only. On Windows run: scripts\install.ps1"; exit 1;; \
	esac
