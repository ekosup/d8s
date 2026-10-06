SHELL := /bin/bash

VERSION := $(shell scripts/version.sh current)
COMMIT  := $(shell scripts/version.sh git-sha)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/ekosup/d8s/internal/version
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

TOOLS         := $(CURDIR)/bin/tools
GOLANGCI      := $(TOOLS)/golangci-lint
GOLANGCI_VER  := v2.14.0
GORELEASER    := $(TOOLS)/goreleaser
GORELEASER_VER := v2.18.2

DEMO_IMAGE := nginx:alpine

.PHONY: build test lint tools version tag demo-up demo-down swarm-up swarm-down test-integration test-matrix bench release-snapshot clean

build: ## build bin/d8s
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/d8s ./cmd/d8s

test: ## unit tests
	go test -race ./...

lint: $(GOLANGCI) ## gofmt + golangci-lint
	@test -z "$$(gofmt -l cmd internal)" || { echo "gofmt needed:"; gofmt -l cmd internal; exit 1; }
	$(GOLANGCI) run ./...

tools: ## install development tools into bin/tools
	GOBIN=$(TOOLS) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VER)
	GOBIN=$(TOOLS) go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VER)

$(GOLANGCI):
	@echo "golangci-lint missing: run 'make tools'"; exit 1

version: ## show current version and git state
	@echo "version: $(VERSION)"
	@echo "commit:  $(COMMIT)"

tag: ## annotated tag v<version>; refuses a dirty tree
	@test -z "$$(git status --porcelain)" || { echo "working tree is dirty"; exit 1; }
	git tag -a "v$(VERSION)" -m "v$(VERSION)"

DEMO_COMPOSE := docker compose -f test/fixtures/demo/compose.yml

demo-up: ## create d8s-demo* containers, network and volume for manual checks
	DEMO_IMAGE=$(DEMO_IMAGE) $(DEMO_COMPOSE) up -d

demo-down: ## remove everything demo-up created
	DEMO_IMAGE=$(DEMO_IMAGE) $(DEMO_COMPOSE) down --volumes

swarm-up: ## three-node swarm in docker-in-docker, context d8s-swarm, sample stack
	scripts/swarm.sh up

swarm-down: ## remove the development swarm
	scripts/swarm.sh down

test-integration: ## tests against the development swarm (run swarm-up first)
	DOCKER_CONTEXT=d8s-swarm go test -tags integration -count=1 ./internal/docker/...

test-matrix: ## integration tests against the oldest supported and the latest engine
	scripts/matrix.sh

bench: ## measure against the performance targets, with synthetic data
	@set -o pipefail; go test -tags bench -count=1 -v -run TestPerformanceTargets ./internal/ui/ | grep -vE '^(=== RUN|--- PASS|PASS$$|ok )'

release-snapshot: ## build every release artefact into dist/, publishing nothing
	@test -x $(GORELEASER) || { echo "goreleaser missing: run 'make tools'"; exit 1; }
	D8S_VERSION=$(VERSION) $(GORELEASER) release --snapshot --clean --skip=publish

clean:
	rm -rf bin/d8s dist
