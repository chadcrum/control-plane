#!/usr/bin/env bash
set -euo pipefail

# Verify schema negative tests for DCM Helm chart
# This script validates that the schema properly rejects invalid configurations.
# Usage: verify-schema.sh <chart-dir> [defaults to deploy/helm/dcm]

chart_dir="${1:-deploy/helm/dcm}"
schema_file="$chart_dir/values.schema.json"

expect_lint_fail() {
	local desc="$1"
	shift
	# Run helm lint with provided --set args and assert non-zero exit
	if helm lint "$chart_dir" "$@" 2>&1 >/dev/null; then
		echo "FAIL: $desc — expected lint to fail but passed" >&2
		exit 1
	else
		echo "PASS: $desc (lint correctly failed)"
	fi
}

expect_lint_pass() {
	local desc="$1"
	shift
	if helm lint "$chart_dir" "$@" 2>&1 >/dev/null; then
		echo "PASS: $desc"
	else
		echo "FAIL: $desc — expected lint to pass but failed" >&2
		exit 1
	fi
}

echo "Verify schema negative tests for chart: $chart_dir"
echo "Schema file: $schema_file"
echo ""

# Positive: auth enabled with default inline credentials
expect_lint_pass "auth.enabled=true with inline credentials" \
	--set 'auth.enabled=true'

# Test (a): Missing proxySecret and auth secret ref when auth.enabled=true
expect_lint_fail "auth.enabled=true without proxySecret and authSecretRef" \
	--set 'auth.enabled=true,auth.proxySecret=,auth.authSecretRef='

# Test (b): route enabled without issuerURL (authSecretRef bypasses inline credential requirement)
expect_lint_fail "keycloak.route.enabled=true without issuerURL" \
	--set 'auth.enabled=true,auth.authSecretRef=my-secret,auth.keycloak.route.enabled=true,auth.issuerURL='

# Test (c): ingress enabled without issuerURL
expect_lint_fail "keycloak.ingress.enabled=true without issuerURL" \
	--set 'auth.enabled=true,auth.authSecretRef=my-secret,auth.keycloak.ingress.enabled=true,auth.issuerURL='

# Test (e): auth disabled - keycloak route/ingress flags do not require issuerURL
expect_lint_pass "auth.enabled=false with keycloak.route.enabled=true and no issuerURL" \
	--set 'auth.enabled=false,auth.keycloak.route.enabled=true,auth.issuerURL='

expect_lint_pass "auth.enabled=false with keycloak.ingress.enabled=true and no issuerURL" \
	--set 'auth.enabled=false,auth.keycloak.ingress.enabled=true,auth.issuerURL='

# Test (d): ingress tls uses Kubernetes hosts array, not singular host
expect_lint_fail "ingress tls with singular host field" \
	--set 'controlPlane.ingress.tls[0].host=example.com,controlPlane.ingress.tls[0].secretName=example-tls'

expect_lint_pass "ingress tls with hosts array" \
	--set 'controlPlane.ingress.tls[0].hosts[0]=example.com,controlPlane.ingress.tls[0].secretName=example-tls'

# Test (f): nested securityContext objects reject unknown properties
expect_lint_fail "capabilities typo dropp instead of drop" \
	--set 'controlPlane.securityContext.capabilities.dropp[0]=ALL'

expect_lint_fail "seccompProfile typo typ instead of type" \
	--set 'controlPlane.securityContext.seccompProfile.typ=RuntimeDefault'

echo ""
echo "All schema negative tests passed!"
