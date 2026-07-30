SHELL := /bin/bash
GO ?= go
NPM ?= npm
COMPOSE ?= docker compose
USER_WEB_DIR := web/user
ADMIN_WEB_DIR := web/admin
DEV_COMPOSE_FILE := deployments/docker-compose/docker-compose.local.yml
MGSCTL_OUTPUT ?= ./bin/mgsctl
MGSCTL_VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo dev)
MGSCTL_COMMIT ?= $(shell git rev-parse --verify HEAD 2>/dev/null || echo unknown)
MGSCTL_BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
MGSCTL_DIRTY ?= $(shell test -z "$$(git status --porcelain --untracked-files=normal 2>/dev/null)" && echo false || echo true)
MGSCTL_LDFLAGS := -s -w -X main.version=$(MGSCTL_VERSION) -X main.commit=$(MGSCTL_COMMIT) -X main.buildTime=$(MGSCTL_BUILD_TIME) -X main.dirty=$(MGSCTL_DIRTY)

.PHONY: dev worker mgsctl test lint openapi compose-up compose-fullstack-up compose-middleware-up compose-down compose-clean user-web-install admin-web-install user-web-dev admin-web-dev

dev:
	$(GO) run ./cmd/api

worker:
	$(GO) run ./cmd/worker

mgsctl:
	@mkdir -p "$(dir $(MGSCTL_OUTPUT))"
	$(GO) build -trimpath -ldflags "$(MGSCTL_LDFLAGS)" -o "$(MGSCTL_OUTPUT)" ./cmd/mgsctl

test:
	$(GO) test ./...

lint:
	$(GO) vet ./...

openapi:
	@test -f api/openapi/openapi.yaml && echo "OpenAPI source ready: api/openapi/openapi.yaml"

compose-up:
	./scripts/dev/up.sh

compose-fullstack-up:
	./scripts/dev/up.sh

compose-middleware-up:
	./scripts/dev/up.sh middleware

compose-down:
	./scripts/dev/down.sh

compose-clean:
	./scripts/dev/down.sh --volumes

user-web-install:
	cd $(USER_WEB_DIR) && $(NPM) install

admin-web-install:
	cd $(ADMIN_WEB_DIR) && $(NPM) install

user-web-dev:
	cd $(USER_WEB_DIR) && $(NPM) run dev

admin-web-dev:
	cd $(ADMIN_WEB_DIR) && $(NPM) run dev
