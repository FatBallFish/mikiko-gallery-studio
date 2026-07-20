SHELL := /bin/bash
GO ?= go
NPM ?= npm
COMPOSE ?= docker compose
USER_WEB_DIR := web/user
ADMIN_WEB_DIR := web/admin
DEV_COMPOSE_FILE := deployments/docker-compose/docker-compose.local.yml

.PHONY: dev worker test lint openapi compose-up compose-fullstack-up compose-middleware-up compose-down compose-clean service-install service-uninstall service-start service-stop service-restart service-status service-logs local-build local-up user-web-install admin-web-install user-web-dev admin-web-dev

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

compose-fullstack-up:
	./scripts/dev/up.sh

compose-middleware-up:
	./scripts/dev/up.sh middleware

compose-down:
	./scripts/dev/down.sh

compose-clean:
	./scripts/dev/down.sh --volumes

service-install:
	./scripts/service/manage.sh install --user

service-uninstall:
	./scripts/service/manage.sh uninstall --user

service-start:
	./scripts/service/manage.sh start --user

service-stop:
	./scripts/service/manage.sh stop --user

service-restart:
	./scripts/service/manage.sh restart --user

service-status:
	./scripts/service/manage.sh status --user

service-logs:
	./scripts/service/manage.sh logs --user

local-build:
	./scripts/local/pgctl.sh build

local-up:
	./scripts/local/pgctl.sh up --background

user-web-install:
	cd $(USER_WEB_DIR) && $(NPM) install

admin-web-install:
	cd $(ADMIN_WEB_DIR) && $(NPM) install

user-web-dev:
	cd $(USER_WEB_DIR) && $(NPM) run dev

admin-web-dev:
	cd $(ADMIN_WEB_DIR) && $(NPM) run dev
