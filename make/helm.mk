HELM_CHART_DIR := deploy/helm/dcm
HELM_VALUES := $(HELM_CHART_DIR)/values.yaml
HELM_VERIFY_TEMPLATE := $(HELM_CHART_DIR)/scripts/verify-template.sh
KEYCLOAK_REALM_SRC := deploy/keycloak/realm-export.json
HELM_REALM_DEST := $(HELM_CHART_DIR)/files/realm-export.json

helm-chart-sync:
	@test -f $(KEYCLOAK_REALM_SRC) || (echo "missing $(KEYCLOAK_REALM_SRC)" >&2; exit 1)
	mkdir -p $(HELM_CHART_DIR)/files
	cp $(KEYCLOAK_REALM_SRC) $(HELM_REALM_DEST)

helm-chart-verify-sync:
	@test -f $(HELM_REALM_DEST) || (echo "missing $(HELM_REALM_DEST): run 'make helm-chart-sync'" >&2; exit 1)
	@cmp -s $(KEYCLOAK_REALM_SRC) $(HELM_REALM_DEST) || (echo "stale $(HELM_REALM_DEST): run 'make helm-chart-sync'" >&2; exit 1)

helm-chart-verify-admin-subject:
	@command -v jq >/dev/null || { echo "jq is required for helm-chart-verify-admin-subject" >&2; exit 1; }; \
	command -v yq >/dev/null || { echo "yq is required for helm-chart-verify-admin-subject" >&2; exit 1; }; \
	realm_id=$$(jq -r '.users[] | select(.username=="dcm-admin") | .id' $(KEYCLOAK_REALM_SRC)); \
	values_id=$$(yq '.auth.adminSubject' $(HELM_VALUES)); \
	test -n "$$realm_id" && test "$$realm_id" != "null" || { echo "missing dcm-admin user id in $(KEYCLOAK_REALM_SRC)" >&2; exit 1; }; \
	test -n "$$values_id" || { echo "missing auth.adminSubject in $(HELM_VALUES)" >&2; exit 1; }; \
	test "$$realm_id" = "$$values_id" || { echo "auth.adminSubject ($$values_id) must match dcm-admin user id in $(KEYCLOAK_REALM_SRC) ($$realm_id)" >&2; exit 1; }

helm-chart-verify: helm-chart-verify-sync helm-chart-verify-admin-subject

helm-chart-lint: helm-chart-verify
	helm lint $(HELM_CHART_DIR)

helm-chart-template: helm-chart-verify
	$(HELM_VERIFY_TEMPLATE) $(HELM_CHART_DIR)

helm-chart-check: helm-chart-verify helm-chart-lint helm-chart-template

.PHONY: helm-chart-sync helm-chart-verify-sync helm-chart-verify-admin-subject helm-chart-verify helm-chart-lint helm-chart-template helm-chart-check
