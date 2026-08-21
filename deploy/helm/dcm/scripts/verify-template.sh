#!/usr/bin/env bash
set -euo pipefail

chart_dir="${1:?usage: $0 <chart-dir>}"
release="dcm"

fail() {
	echo "$*" >&2
	exit 1
}

helm_out() {
	helm template "$release" "$chart_dir" "$@"
}

yaml_block() {
	local awk_expr="$1"
	printf '%s' "$2" | awk "$awk_expr"
}

require_block() {
	local name="$1"
	local awk_expr="$2"
	shift 2
	local out block
	out="$(helm_out "$@")"
	block="$(yaml_block "$awk_expr" "$out")"
	if [ -z "$block" ]; then
		fail "missing $name"
	fi
	printf '%s' "$block"
}

require_template_failure() {
	local name="$1"
	local msg="$2"
	shift 2
	local out ec=0
	out="$(helm_out "$@" 2>&1)" || ec=$?
	if [ "$ec" -eq 0 ]; then
		fail "expected helm template to fail for $name"
	fi
	if ! printf '%s\n' "$out" | grep -Fq "$msg"; then
		echo "unexpected error for $name:" >&2
		printf '%s\n' "$out" >&2
		exit 1
	fi
}

helm_out --set auth.enabled=false >/dev/null

issuer_url="https://keycloak.example.com/realms/dcm"

block="$(require_block "keycloak-realm manifest in helm template output" 'BEGIN{RS="---"} /keycloak-realm/ {print; exit}' --set auth.enabled=true)"
printf '%s' "$block" | grep -q '^kind: Secret$' || fail "keycloak-realm must render as Secret"

require_block "Keycloak Route manifest when auth.keycloak.route.enabled=true" 'BEGIN{RS="---"} /templates\/keycloak.yaml/ && /kind: Route/ {print; exit}' --set auth.enabled=true --set auth.issuerURL="$issuer_url" --set auth.keycloak.route.enabled=true >/dev/null

require_block "Keycloak Ingress manifest when auth.keycloak.ingress.enabled=true" 'BEGIN{RS="---"} /templates\/keycloak.yaml/ && /kind: Ingress/ {print; exit}' --set auth.enabled=true --set auth.issuerURL="$issuer_url" --set auth.keycloak.ingress.enabled=true >/dev/null

require_template_failure "empty auth credentials" "auth.proxySecret or auth.authSecretRef is required when auth.enabled=true" --set auth.enabled=true --set auth.proxySecret= --set auth.authSecretRef=

require_template_failure "route without auth.issuerURL" "auth.issuerURL is required when auth.keycloak.route.enabled=true" --set auth.enabled=true --set auth.keycloak.route.enabled=true

require_template_failure "invalid auth.issuerURL suffix" "auth.issuerURL must end with /realms/dcm" --set auth.enabled=true --set auth.issuerURL="https://keycloak.example.com"

require_template_failure "ingress without auth.issuerURL" "auth.issuerURL is required when auth.keycloak.ingress.enabled=true" --set auth.enabled=true --set auth.keycloak.ingress.enabled=true

auth_ref_out="$(helm_out --set auth.enabled=true --set auth.authSecretRef=my-auth)"
if printf '%s' "$auth_ref_out" | awk 'BEGIN{RS="---"} /kind: Secret/ && /name: dcm-auth/ {found=1} END{exit !found}'; then
	fail "chart auth Secret must not render when auth.authSecretRef is set"
fi
auth_ref_count="$(printf '%s' "$auth_ref_out" | grep -c 'name: my-auth')"
[ "$auth_ref_count" -eq 4 ] || fail "pods must reference auth.authSecretRef in all 4 secretKeyRef entries (found $auth_ref_count)"

