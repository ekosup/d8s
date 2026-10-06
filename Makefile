SHELL := /bin/bash

VERSION := $(shell scripts/version.sh current)
COMMIT  := $(shell scripts/version.sh git-sha)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/ekosup/d8s/internal/version
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

TOOLS         := $(CURDIR)/bin/tools
GOLANGCI      := $(TOOLS)/golangci-lint
GOLANGCI_VER  := v2.14.0

DEMO_IMAGE := nginx:alpine

.PHONY: build test lint tools version tag demo-up demo-down clean

build: ## build bin/d8s
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/d8s ./cmd/d8s

test: ## unit tests
	go test -race ./...

lint: $(GOLANGCI) ## gofmt + golangci-lint
	@test -z "$$(gofmt -l cmd internal)" || { echo "gofmt needed:"; gofmt -l cmd internal; exit 1; }
	$(GOLANGCI) run ./...

tools: ## install development tools into bin/tools
	GOBIN=$(TOOLS) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VER)

$(GOLANGCI):
	@echo "golangci-lint missing: run 'make tools'"; exit 1

version: ## show current version and git state
	@echo "version: $(VERSION)"
	@echo "commit:  $(COMMIT)"

tag: ## annotated tag v<version>; refuses a dirty tree
	@test -z "$$(git status --porcelain)" || { echo "working tree is dirty"; exit 1; }
	git tag -a "v$(VERSION)" -m "v$(VERSION)"

demo-up: ## create d8s-demo-* containers for manual checks
	@docker rm -f d8s-demo-web d8s-demo-idle >/dev/null 2>&1 || true
	docker run -d --name d8s-demo-web --label d8s.demo=1 $(DEMO_IMAGE)
	docker run -d --name d8s-demo-idle --label d8s.demo=1 $(DEMO_IMAGE) sleep infinity

demo-down: ## remove d8s-demo-* containers
	@ids="$$(docker ps -aq --filter label=d8s.demo=1)"; \
	if [ -n "$$ids" ]; then docker rm -f $$ids; fi

clean:
	rm -rf bin/d8s dist
