BINARY_NAME := control-plane

# CONTAINER_ENGINE: container runtime command. Set to override; otherwise auto-detect podman or docker.
CONTAINER_ENGINE ?= $(shell \
	if command -v podman >/dev/null 2>&1; then \
		echo podman; \
	elif command -v docker >/dev/null 2>&1; then \
		echo docker; \
	fi)

ifeq ($(CONTAINER_ENGINE),)
$(error No supported container engine found. Please install podman or docker, or set CONTAINER_ENGINE explicitly.)
endif

COMPOSE_FILE := deploy/compose.yaml
COMPOSE_ENV_FILE := deploy/.env
COMPOSE_DEPLOY := $(COMPOSE) --env-file $(COMPOSE_ENV_FILE) -f $(COMPOSE_FILE)
COMPOSE_PROJECT_NAME ?= control-plane
COMPOSE_NETWORK := $(COMPOSE_PROJECT_NAME)_default
PROFILES ?=
AUTH ?=

COMPOSE ?= $(shell command -v podman-compose >/dev/null 2>&1 && echo podman-compose || \
	(command -v docker-compose >/dev/null 2>&1 && echo docker-compose || \
	(echo "$(CONTAINER_ENGINE) compose")))

export COMPOSE_PROJECT_NAME

CONTAINER_IMAGE_NAME ?= quay.io/dcm-project/$(BINARY_NAME)
CONTAINER_IMAGE_TAG ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

include make/helm.mk
include make/auth.mk
include make/catalog.mk
include make/placement.mk
include make/policy.mk
include make/sp.mk
include make/agent.mk

# Same as Containerfile: static build, no CGO (Postgres in prod/compose).
# For SQLite local dev use make run (go run with CGO).
build:
	CGO_ENABLED=0 go build -buildvcs=false -o bin/$(BINARY_NAME) ./cmd/$(BINARY_NAME)

# Quick local start with SQLite (no Postgres/NATS stack required).
# Uses a throwaway file under /tmp; override DB_NAME if you want a different path.
run:
	DB_TYPE=sqlite DB_NAME=/tmp/control-plane.db NATS_DISABLED=true go run ./cmd/$(BINARY_NAME)

# Run with config defaults (pgsql + DB_NAME=control-plane). Use with deploy/compose
# or set DB_* / NATS_* yourself.
run-dev:
	go run ./cmd/$(BINARY_NAME)

# Platform stack: Postgres, NATS, control-plane, and dcm-ui (see deploy/compose.yaml).
# Optional: AUTH=true to also start Keycloak (uncomment auth vars in deploy/.env first).
compose-up:
	$(COMPOSE_DEPLOY) $(if $(filter true,$(AUTH)),--profile auth,) up -d --build

# Platform stack with optional service providers (see deploy/RUN.md).
# Optional: AUTH=true to also start Keycloak.
compose-up-with-providers:
	$(COMPOSE_DEPLOY) $(if $(filter true,$(AUTH)),--profile auth,) --profile $(or $(PROFILES),providers) up -d --build

# Tear down the compose stack. Kind (or other externals) joined to the compose
# network block "compose down" from removing it — disconnect them first.
# Network cleanup uses podman- or docker-specific commands (not portable flags).
compose-down:
	@for network in deploy_default $(COMPOSE_NETWORK); do \
		if [ "$(CONTAINER_ENGINE)" = podman ]; then \
			if podman network exists "$$network" 2>/dev/null; then \
				for c in $$(podman ps -a --filter network=$$network -q 2>/dev/null); do \
					podman network disconnect -f "$$network" "$$c" 2>/dev/null || true; \
				done; \
			fi; \
		elif [ "$(CONTAINER_ENGINE)" = docker ]; then \
			if docker network inspect "$$network" >/dev/null 2>&1; then \
				for c in $$(docker ps -a --filter network=$$network -q 2>/dev/null); do \
					docker network disconnect "$$network" "$$c" --force 2>/dev/null || true; \
				done; \
			fi; \
		fi; \
	done; \
	COMPOSE_PROJECT_NAME=deploy $(COMPOSE_DEPLOY) down -v --remove-orphans 2>/dev/null || true; \
	$(COMPOSE_DEPLOY) down -v --remove-orphans; \
	for network in deploy_default $(COMPOSE_NETWORK); do \
		if [ "$(CONTAINER_ENGINE)" = podman ]; then \
			podman network rm -f "$$network" 2>/dev/null || true; \
		elif [ "$(CONTAINER_ENGINE)" = docker ]; then \
			docker network rm "$$network" 2>/dev/null || true; \
		fi; \
	done

image-build:
	$(CONTAINER_ENGINE) build -f Containerfile -t $(CONTAINER_IMAGE_NAME):$(CONTAINER_IMAGE_TAG) .

clean:
	rm -rf bin/

fmt:
	gofmt -s -w .

vet:
	go vet ./...

GOLANGCI_LINT_VERSION ?= v2.12.2

GINKGO := go run github.com/onsi/ginkgo/v2/ginkgo
GINKGO_FLAGS := -r --randomize-all --fail-on-pending

# Subsystem compose stacks read credentials from gitignored test/subsystem/.env.
subsystem-env:
	@test -f test/subsystem/.env || cp test/subsystem/.env.example test/subsystem/.env

auth-subsystem-test-up catalog-subsystem-test-up policy-subsystem-test-up sp-subsystem-test-up: subsystem-env

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

test:
	$(GINKGO) $(GINKGO_FLAGS) --skip-package=test/subsystem

tidy:
	go mod tidy

.PHONY: build run run-dev compose-up compose-up-with-providers compose-down image-build \
	clean fmt vet lint test test-catalog test-placement test-policy test-sp tidy subsystem-env \
	helm-chart-sync helm-chart-verify-sync helm-chart-verify-admin-subject helm-chart-verify helm-chart-verify-schema helm-chart-lint helm-chart-template helm-chart-check
