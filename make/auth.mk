# Auth domain (subsystem tests).
AUTH_DOMAIN := auth

AUTH_COMPOSE = COMPOSE_PROJECT_NAME=auth-subsystem $(COMPOSE) -f test/subsystem/$(AUTH_DOMAIN)/docker-compose.yaml

auth-subsystem-test-up:
	$(AUTH_COMPOSE) up -d --build

auth-subsystem-test-down:
	$(AUTH_COMPOSE) down -v

auth-subsystem-test:
	$(GINKGO) $(GINKGO_FLAGS) -tags=subsystem ./test/subsystem/$(AUTH_DOMAIN)

.PHONY: auth-subsystem-test-up auth-subsystem-test-down auth-subsystem-test
