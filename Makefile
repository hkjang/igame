SHELL := /bin/bash
.DEFAULT_GOAL := help

VERSION := $(shell tr -d '[:space:]' < VERSION)
IMAGE := igame:v$(VERSION)
COMMIT := $(shell git rev-parse --short=12 HEAD 2>/dev/null || printf unknown)
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

.PHONY: help deps fmt lint test sdk-build web-build check-offline-bundle build docker-build smoke realmguard-smoke release verify-release check-contract clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "; printf "igame build targets:\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

deps: ## Install locked Go and Node dependencies
	go mod download
	npm --prefix sdk/gamehub-js ci
	npm --prefix web ci

fmt: ## Format Go source
	gofmt -w $$(find . -name '*.go' -not -path './web/*' -not -path './sdk/*')

lint: ## Run Go vet and frontend linters
	@unformatted="$$(gofmt -l $$(find cmd internal migrations -name '*.go' -type f))"; \
	  if [[ -n "$$unformatted" ]]; then printf 'Go files need gofmt:\n%s\n' "$$unformatted" >&2; exit 1; fi
	go vet ./cmd/... ./internal/... ./migrations/...
	npm --prefix sdk/gamehub-js run lint
	npm --prefix web run lint

test: ## Run Go, SDK, and frontend tests
	go test ./cmd/... ./internal/... ./migrations/...
	npm --prefix sdk/gamehub-js test
	npm --prefix web test

sdk-build: ## Build the JavaScript Game SDK
	npm --prefix sdk/gamehub-js run build

web-build: sdk-build ## Build the React portal
	npm --prefix web run build
	bash ./scripts/check-offline-bundle.sh

check-offline-bundle: web-build ## Verify Phaser and RealmGuard are bundled without remote assets

build: web-build ## Build the versioned Go binary
	bash ./scripts/build-local.sh "$(VERSION)" "$(COMMIT)" "$(BUILD_DATE)"

check-contract: ## Check version and runtime environment contracts
	bash ./scripts/check-release-contract.sh

docker-build: check-contract ## Build the single offline image
	docker build \
	  --platform linux/amd64 \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg COMMIT=$(COMMIT) \
	  --build-arg BUILD_DATE=$(BUILD_DATE) \
	  --tag $(IMAGE) .

smoke: ## Smoke-test a running service; override URL=...
	bash ./scripts/smoke-test.sh "$(or $(URL),http://127.0.0.1:8080)"

realmguard-smoke: ## Test RealmGuard on a disposable fresh service; set ADMIN and PASSWORD
	bash ./scripts/smoke-realmguard.sh "$(or $(URL),http://127.0.0.1:8080)" "$(ADMIN)" "$(PASSWORD)"

release: check-contract ## Build the single release asset plus local SBOM/checksum evidence
	bash ./scripts/release.sh

verify-release: ## Verify the release archive; override ARCHIVE=...
	bash ./scripts/verify-release.sh "$(or $(ARCHIVE),dist/igame-v$(VERSION).tar.gz)"

clean: ## Remove generated local build output
	rm -rf bin dist web/dist sdk/gamehub-js/dist
