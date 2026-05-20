SHELL := /bin/bash
GO ?= go
NPM ?= npm
COMPOSE ?= docker compose
USER_WEB_DIR := web/user
ADMIN_WEB_DIR := web/admin
DEV_COMPOSE_FILE := deployments/docker-compose/docker-compose.dev.yml

.PHONY: dev worker test lint openapi compose-up compose-down user-web-install admin-web-install user-web-dev admin-web-dev

dev:
	$(GO) run ./cmd/api

worker:
	$(GO) run ./cmd/worker

test:
	$(GO) test ./...

lint:
	$(GO) vet ./...

openapi:
	@test -f api/openapi/openapi.yaml && echo "OpenAPI source ready: api/openapi/openapi.yaml"

compose-up:
	./scripts/dev/up.sh

compose-down:
	./scripts/dev/down.sh

user-web-install:
	cd $(USER_WEB_DIR) && $(NPM) install

admin-web-install:
	cd $(ADMIN_WEB_DIR) && $(NPM) install

user-web-dev:
	cd $(USER_WEB_DIR) && $(NPM) run dev

admin-web-dev:
	cd $(ADMIN_WEB_DIR) && $(NPM) run dev
