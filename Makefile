.PHONY: help all build test test-race test-coverage test-verbose lint fmt fmt-check vet clean deps tidy check install-tools benchmark build-ui build-full setup-tauri build-tauri dev-tauri prepare-sidecar prepare-tauri-sidecar release-tauri-macos notarize-tauri-macos staple-tauri-macos verify-tauri-macos

# Variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOVET=$(GOCMD) vet
BINARY_NAME=anyclaw
BUILD_DIR=.
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
DOCKER_IMAGE=anyclaw
DOCKER_TAG=$(VERSION)
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html
GO_CACHE_DIR=$(CURDIR)/.gocache
MACOS_SIGNING_IDENTITY?=
APPLE_ID?=
APPLE_PASSWORD?=
APPLE_TEAM_ID?=
TAURI_BUNDLE_DIR=$(CURDIR)/src-tauri/target/release/bundle
TAURI_MACOS_APP=$(firstword $(wildcard $(TAURI_BUNDLE_DIR)/macos/*.app))
TAURI_MACOS_DMG=$(firstword $(wildcard $(TAURI_BUNDLE_DIR)/dmg/*.dmg))

# Colors for terminal output
COLOR_RESET=\033[0m
COLOR_BOLD=\033[1m
COLOR_GREEN=\033[32m
COLOR_YELLOW=\033[33m
COLOR_BLUE=\033[34m

# Default target
all: clean fmt lint test build

## help: Display this help message
help:
	@echo "$(COLOR_BOLD)AnyClaw - Makefile Commands$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)Usage:$(COLOR_RESET)"
	@echo "  make $(COLOR_GREEN)<target>$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)Available targets:$(COLOR_RESET)"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  $(COLOR_GREEN)/' | sed 's/:/ $(COLOR_RESET)-/'
	@echo ""

## build: Build the project
build:
	@echo "$(COLOR_BLUE)Building $(BINARY_NAME)...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	@mkdir -p $(GO_CACHE_DIR)
	GOCACHE=$(GO_CACHE_DIR) $(GOBUILD) -buildvcs=false -ldflags="-X 'main.Version=$(VERSION)'" -o $(BUILD_DIR)/$(BINARY_NAME) .

## build-ui: Build the UI frontend
build-ui:
	@echo "$(COLOR_BLUE)Building UI frontend...$(COLOR_RESET)"
	@cd ui && npm install && npm run build
	@echo "$(COLOR_GREEN)UI built successfully$(COLOR_RESET)"

## build-full: Build UI and then build the binary (embeds UI)
build-full: build-ui
	@echo "$(COLOR_BLUE)Copying UI to gateway/ui_dist...$(COLOR_RESET)"
	@rm -rf gateway/ui_dist && cp -r ui/dist gateway/ui_dist
	@echo "$(COLOR_BLUE)Building $(BINARY_NAME) with embedded UI...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	@mkdir -p $(GO_CACHE_DIR)
	GOCACHE=$(GO_CACHE_DIR) $(GOBUILD) -buildvcs=false -ldflags="-X 'main.Version=$(VERSION)'" -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "$(COLOR_GREEN)Build complete! Binary: $(BUILD_DIR)/$(BINARY_NAME)$(COLOR_RESET)"

## test: Run all tests
test:
	@echo "$(COLOR_BLUE)Running tests...$(COLOR_RESET)"
	$(GOTEST) -v ./...

## test-short: Run tests in short mode
test-short:
	@echo "$(COLOR_BLUE)Running tests (short mode)...$(COLOR_RESET)"
	$(GOTEST) -short ./...

## test-race: Run tests with race detector
test-race:
	@echo "$(COLOR_BLUE)Running tests with race detector...$(COLOR_RESET)"
	$(GOTEST) -race ./...

## test-coverage: Run tests with coverage report
test-coverage:
	@echo "$(COLOR_BLUE)Running tests with coverage...$(COLOR_RESET)"
	$(GOTEST) -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@echo "$(COLOR_GREEN)Coverage report generated: $(COVERAGE_FILE)$(COLOR_RESET)"
	$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "$(COLOR_GREEN)HTML coverage report: $(COVERAGE_HTML)$(COLOR_RESET)"

## test-verbose: Run tests with verbose output
test-verbose:
	@echo "$(COLOR_BLUE)Running tests (verbose)...$(COLOR_RESET)"
	$(GOTEST) -v -count=1 ./...

## benchmark: Run benchmarks
benchmark:
	@echo "$(COLOR_BLUE)Running benchmarks...$(COLOR_RESET)"
	$(GOTEST) -bench=. -benchmem ./...

## lint: Run golangci-lint
lint:
	@echo "$(COLOR_BLUE)Running linter...$(COLOR_RESET)"
	@which golangci-lint > /dev/null || (echo "$(COLOR_YELLOW)golangci-lint not found. Run 'make install-tools'$(COLOR_RESET)" && exit 1)
	golangci-lint run ./...

## lint-fix: Auto-fix lint issues
lint-fix:
	@echo "$(COLOR_BLUE)Auto-fixing lint issues...$(COLOR_RESET)"
	@which golangci-lint > /dev/null || (echo "$(COLOR_YELLOW)golangci-lint not found. Run 'make install-tools'$(COLOR_RESET)" && exit 1)
	golangci-lint run --fix ./...
	@echo "$(COLOR_GREEN)Lint fixes applied$(COLOR_RESET)"

## fmt: Format all Go files
fmt:
	@echo "$(COLOR_BLUE)Formatting code...$(COLOR_RESET)"
	$(GOFMT) -s -w .
	@echo "$(COLOR_GREEN)Code formatted successfully$(COLOR_RESET)"

## fmt-check: Check if code is formatted
fmt-check:
	@echo "$(COLOR_BLUE)Checking code formatting...$(COLOR_RESET)"
	@test -z "$$($(GOFMT) -l .)" || (echo "$(COLOR_YELLOW)The following files need formatting:$(COLOR_RESET)" && $(GOFMT) -l . && exit 1)
	@echo "$(COLOR_GREEN)All files are properly formatted$(COLOR_RESET)"

## vet: Run go vet
vet:
	@echo "$(COLOR_BLUE)Running go vet...$(COLOR_RESET)"
	$(GOVET) ./...

## check: Run fmt-check, vet, and lint
check: fmt-check vet lint
	@echo "$(COLOR_GREEN)All checks passed!$(COLOR_RESET)"

## clean: Clean build artifacts and test cache
clean:
	@echo "$(COLOR_BLUE)Cleaning...$(COLOR_RESET)"
	$(GOCLEAN)
	rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	rm -rf ui/dist
	rm -rf gateway/ui_dist
	@echo "$(COLOR_GREEN)Clean complete$(COLOR_RESET)"

## deps: Download dependencies
deps:
	@echo "$(COLOR_BLUE)Downloading dependencies...$(COLOR_RESET)"
	$(GOMOD) download
	@echo "$(COLOR_GREEN)Dependencies downloaded$(COLOR_RESET)"

## tidy: Tidy and verify dependencies
tidy:
	@echo "$(COLOR_BLUE)Tidying dependencies...$(COLOR_RESET)"
	$(GOMOD) tidy
	$(GOMOD) verify
	@echo "$(COLOR_GREEN)Dependencies tidied$(COLOR_RESET)"

## install-tools: Install development tools
install-tools:
	@echo "$(COLOR_BLUE)Installing development tools...$(COLOR_RESET)"
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	@echo "$(COLOR_GREEN)Tools installed$(COLOR_RESET)"

## run: Run the application
run:
	@echo "$(COLOR_BLUE)Running $(BINARY_NAME)...$(COLOR_RESET)"
	$(GOCMD) run .

## install: Install the binary to GOPATH/bin
install:
	@echo "$(COLOR_BLUE)Installing $(BINARY_NAME)...$(COLOR_RESET)"
	$(GOCMD) install

## docs: Generate documentation
docs:
	@echo "$(COLOR_BLUE)Generating documentation...$(COLOR_RESET)"
	@echo "Open http://localhost:6060/pkg/anyclaw/ in your browser"
	godoc -http=:6060

## ci: Run continuous integration checks
ci: deps check test-race test-coverage
	@echo "$(COLOR_GREEN)CI checks passed!$(COLOR_RESET)"

## pre-commit: Run pre-commit checks (fmt, vet, lint, test)
pre-commit: fmt vet lint test
	@echo "$(COLOR_GREEN)Pre-commit checks passed!$(COLOR_RESET)"

## update-deps: Update all dependencies to latest versions
update-deps:
	@echo "$(COLOR_BLUE)Updating dependencies...$(COLOR_RESET)"
	$(GOGET) -u ./...
	$(GOMOD) tidy
	@echo "$(COLOR_GREEN)Dependencies updated$(COLOR_RESET)"

## version: Display Go version
version:
	@$(GOCMD) version

## info: Display project information
info:
	@echo "$(COLOR_BOLD)Project Information$(COLOR_RESET)"
	@echo "  Name: AnyClaw"
	@echo "  Module: anyclaw/"
	@echo "  Go Version: $$($(GOCMD) version | cut -d' ' -f3)"
	@echo "  Version: $(VERSION)"
	@echo "  Packages: $$(find . -name '*.go' -not -path './vendor/*' | xargs dirname | sort -u | wc -l | tr -d ' ')"
	@echo "  Lines of Code: $$(find . -name '*.go' -not -path './vendor/*' | xargs wc -l | tail -1 | awk '{print $$1}')"

## setup: Setup development environment
setup:
	@echo "$(COLOR_BLUE)Setting up development environment...$(COLOR_RESET)"
	@mkdir -p .anyclaw/workspace .anyclaw/sessions
	@cp .env.example .env 2>/dev/null || echo "Please copy .env.example to .env and configure"
	@echo "$(COLOR_GREEN)Setup complete. Edit .env with your configuration.$(COLOR_RESET)"

# Docker targets
## docker-build: Build Docker image
docker-build:
	@echo "$(COLOR_BLUE)Building Docker image...$(COLOR_RESET)"
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_IMAGE):latest
	@echo "$(COLOR_GREEN)Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)$(COLOR_RESET)"

## docker-run: Run Docker container
docker-run:
	@echo "$(COLOR_BLUE)Running Docker container...$(COLOR_RESET)"
	docker run --rm -it \
		-p 8080:8080 \
		-v $(PWD)/config.json:/home/anyclaw/.anyclaw/config.json:ro \
		$(DOCKER_IMAGE):latest

## docker-compose-up: Start services with docker-compose
docker-compose-up:
	@echo "$(COLOR_BLUE)Starting services with docker-compose...$(COLOR_RESET)"
	docker-compose up -d

## docker-compose-down: Stop services
docker-compose-down:
	@echo "$(COLOR_BLUE)Stopping services...$(COLOR_RESET)"
	docker-compose down

## docker-compose-logs: Show logs from services
docker-compose-logs:
	@echo "$(COLOR_BLUE)Showing logs...$(COLOR_RESET)"
	docker-compose logs -f

## docker-compose-ps: Show running services
docker-compose-ps:
	@echo "$(COLOR_BLUE)Showing running services...$(COLOR_RESET)"
	docker-compose ps

## docker-shell: Open shell in container
docker-shell:
	@echo "$(COLOR_BLUE)Opening shell in container...$(COLOR_RESET)"
	docker-compose exec anyclaw sh

## dev: Start development environment
dev: docker-compose-up docker-compose-logs

## release-check: Check goreleaser configuration
release-check:
	@which goreleaser > /dev/null || (echo "goreleaser not found. Install with: brew install goreleaser" && exit 1)
	@echo "$(COLOR_BLUE)Checking goreleaser configuration...$(COLOR_RESET)"
	goreleaser check

## release-snapshot: Build snapshot release (no publishing)
release-snapshot:
	@which goreleaser > /dev/null || (echo "goreleaser not found. Install with: brew install goreleaser" && exit 1)
	@echo "$(COLOR_BLUE)Building snapshot release...$(COLOR_RESET)"
	goreleaser build --snapshot --clean
	@echo "$(COLOR_GREEN)Snapshot release built in dist/$(COLOR_RESET)"

## release-test: Test goreleaser release process (no publishing)
release-test:
	@which goreleaser > /dev/null || (echo "goreleaser not found. Install with: brew install goreleaser" && exit 1)
	@echo "$(COLOR_BLUE)Testing goreleaser release...$(COLOR_RESET)"
	goreleaser release --snapshot --clean --skip=publish
	@echo "$(COLOR_GREEN)Release test complete. Artifacts in dist/$(COLOR_RESET)"

## release: Create and publish a new release (requires tag)
release:
	@echo "$(COLOR_YELLOW)To create a release:$(COLOR_RESET)"
	@echo "  1. Create a git tag: git tag v1.0.0"
	@echo "  2. Push the tag: git push origin v1.0.0"
	@echo "  3. GitHub Actions will automatically build and publish"

## release-notes: Generate release notes
release-notes:
	@which goreleaser > /dev/null || (echo "goreleaser not found. Install with: brew install goreleaser" && exit 1)
	@echo "$(COLOR_BLUE)Generating release notes...$(COLOR_RESET)"
	goreleaser release --release-notes=release-notes.txt --skip=publish --skip=validate --skip=announce

# Tauri Desktop App targets
# Helper to get Rust target triple
TAURI_TARGET=$(shell rustc -vV | grep host | cut -d' ' -f2)

## setup-tauri: Install Tauri CLI and dependencies
setup-tauri:
	@echo "$(COLOR_BLUE)Installing Tauri CLI...$(COLOR_RESET)"
	@which cargo > /dev/null || (echo "$(COLOR_YELLOW)Rust/Cargo not found. Install from https://rustup.rs$(COLOR_RESET)" && exit 1)
	cargo install tauri-cli --version "^1.5" || echo "Tauri CLI already installed"
	@echo "$(COLOR_GREEN)Tauri setup complete$(COLOR_RESET)"
	@echo "Run 'make dev-tauri' to start development mode"
	@echo "Run 'make build-tauri' to build the desktop app"

## prepare-sidecar: Build and prepare the anyclaw binary for sidecar
prepare-sidecar: build-full
	@echo "$(COLOR_BLUE)Preparing sidecar binary...$(COLOR_RESET)"
	@mkdir -p src-tauri/binaries
	@cp $(BINARY_NAME) src-tauri/binaries/anyclaw-$(TAURI_TARGET)
	@echo "$(COLOR_GREEN)Sidecar binary prepared$(COLOR_RESET)"

## prepare-tauri-sidecar: Build embedded UI and sidecar binary for the current platform
prepare-tauri-sidecar: prepare-sidecar
	@echo "$(COLOR_GREEN)Tauri sidecar is ready$(COLOR_RESET)"

## dev-tauri: Start Tauri development mode
dev-tauri:
	@echo "$(COLOR_BLUE)Starting Tauri development mode...$(COLOR_RESET)"
	@which cargo > /dev/null || (echo "$(COLOR_YELLOW)Rust/Cargo not found. Install from https://rustup.rs$(COLOR_RESET)" && exit 1)
	@$(MAKE) prepare-tauri-sidecar
	cargo tauri dev

## build-tauri: Build Tauri desktop application
build-tauri:
	@echo "$(COLOR_BLUE)Building Tauri desktop application...$(COLOR_RESET)"
	@which cargo > /dev/null || (echo "$(COLOR_YELLOW)Rust/Cargo not found. Install from https://rustup.rs$(COLOR_RESET)" && exit 1)
	@$(MAKE) build-full
	@# Create sidecar binaries for all platforms (for release builds)
	@mkdir -p src-tauri/binaries
	@echo "$(COLOR_BLUE)Creating sidecar binaries...$(COLOR_RESET)"
	@# For macOS (Intel and Apple Silicon)
	@mkdir -p $(GO_CACHE_DIR)
	GOCACHE=$(GO_CACHE_DIR) GOOS=darwin GOARCH=amd64 $(GOBUILD) -buildvcs=false -ldflags="-X 'main.Version=$(VERSION)'" -o src-tauri/binaries/anyclaw-x86_64-apple-darwin .
	GOCACHE=$(GO_CACHE_DIR) GOOS=darwin GOARCH=arm64 $(GOBUILD) -buildvcs=false -ldflags="-X 'main.Version=$(VERSION)'" -o src-tauri/binaries/anyclaw-aarch64-apple-darwin .
	@# For Linux
	GOCACHE=$(GO_CACHE_DIR) GOOS=linux GOARCH=amd64 $(GOBUILD) -buildvcs=false -ldflags="-X 'main.Version=$(VERSION)'" -o src-tauri/binaries/anyclaw-x86_64-unknown-linux-gnu .
	@# For Windows
	GOCACHE=$(GO_CACHE_DIR) GOOS=windows GOARCH=amd64 $(GOBUILD) -buildvcs=false -ldflags="-X 'main.Version=$(VERSION)'" -o src-tauri/binaries/anyclaw-x86_64-pc-windows-msvc.exe .
	cargo tauri build
	@echo "$(COLOR_GREEN)Tauri build complete!$(COLOR_RESET)"
	@echo "Find the application in src-tauri/target/release/bundle/"

## build-tauri-current: Build Tauri app for current platform only (faster)
build-tauri-current:
	@echo "$(COLOR_BLUE)Building Tauri for current platform...$(COLOR_RESET)"
	@which cargo > /dev/null || (echo "$(COLOR_YELLOW)Rust/Cargo not found. Install from https://rustup.rs$(COLOR_RESET)" && exit 1)
	@$(MAKE) prepare-tauri-sidecar
	cargo tauri build
	@echo "$(COLOR_GREEN)Tauri build complete for current platform$(COLOR_RESET)"

## release-tauri-macos: Build signed macOS release bundles (.app/.dmg)
release-tauri-macos:
	@echo "$(COLOR_BLUE)Building signed macOS Tauri bundle...$(COLOR_RESET)"
	@test -n "$(MACOS_SIGNING_IDENTITY)" || (echo "$(COLOR_YELLOW)Set MACOS_SIGNING_IDENTITY first$(COLOR_RESET)" && exit 1)
	@$(MAKE) prepare-tauri-sidecar
	cargo tauri build --config '{"tauri":{"bundle":{"macOS":{"signingIdentity":"$(MACOS_SIGNING_IDENTITY)"}}}}'
	@echo "$(COLOR_GREEN)Signed macOS bundle built$(COLOR_RESET)"
	@echo "App: $(TAURI_BUNDLE_DIR)/macos/"
	@echo "DMG: $(TAURI_BUNDLE_DIR)/dmg/"

## notarize-tauri-macos: Submit built DMG to Apple notarization
notarize-tauri-macos:
	@echo "$(COLOR_BLUE)Submitting DMG for notarization...$(COLOR_RESET)"
	@test -n "$(APPLE_ID)" || (echo "$(COLOR_YELLOW)Set APPLE_ID first$(COLOR_RESET)" && exit 1)
	@test -n "$(APPLE_PASSWORD)" || (echo "$(COLOR_YELLOW)Set APPLE_PASSWORD first$(COLOR_RESET)" && exit 1)
	@test -n "$(APPLE_TEAM_ID)" || (echo "$(COLOR_YELLOW)Set APPLE_TEAM_ID first$(COLOR_RESET)" && exit 1)
	@test -n "$(TAURI_MACOS_DMG)" || (echo "$(COLOR_YELLOW)No DMG found under $(TAURI_BUNDLE_DIR)/dmg$(COLOR_RESET)" && exit 1)
	xcrun notarytool submit "$(TAURI_MACOS_DMG)" --apple-id "$(APPLE_ID)" --password "$(APPLE_PASSWORD)" --team-id "$(APPLE_TEAM_ID)" --wait
	@echo "$(COLOR_GREEN)Notarization completed$(COLOR_RESET)"

## staple-tauri-macos: Staple notarization ticket to built app and DMG
staple-tauri-macos:
	@echo "$(COLOR_BLUE)Stapling notarization ticket...$(COLOR_RESET)"
	@test -n "$(TAURI_MACOS_APP)" || (echo "$(COLOR_YELLOW)No app found under $(TAURI_BUNDLE_DIR)/macos$(COLOR_RESET)" && exit 1)
	@test -n "$(TAURI_MACOS_DMG)" || (echo "$(COLOR_YELLOW)No DMG found under $(TAURI_BUNDLE_DIR)/dmg$(COLOR_RESET)" && exit 1)
	xcrun stapler staple "$(TAURI_MACOS_APP)"
	xcrun stapler staple "$(TAURI_MACOS_DMG)"
	@echo "$(COLOR_GREEN)Stapling completed$(COLOR_RESET)"

## verify-tauri-macos: Verify app signature and Gatekeeper assessment
verify-tauri-macos:
	@echo "$(COLOR_BLUE)Verifying macOS bundle...$(COLOR_RESET)"
	@test -n "$(TAURI_MACOS_APP)" || (echo "$(COLOR_YELLOW)No app found under $(TAURI_BUNDLE_DIR)/macos$(COLOR_RESET)" && exit 1)
	codesign -dv --verbose=4 "$(TAURI_MACOS_APP)"
	spctl -a -vv "$(TAURI_MACOS_APP)"
	@if [ -n "$(TAURI_MACOS_DMG)" ]; then spctl -a -vv "$(TAURI_MACOS_DMG)"; fi
	@echo "$(COLOR_GREEN)Verification completed$(COLOR_RESET)"
