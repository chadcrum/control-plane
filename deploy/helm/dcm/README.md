# DCM Helm Chart

## Prerequisites

- Kubernetes 1.24+ or OpenShift 4.12+
- Helm 3.x
- A default StorageClass configured in the cluster (for PostgreSQL and NATS persistent volumes)

## Quick Start

Install all the components with a kubernetes provider using default namespace.

### OpenShift

```bash
helm install dcm deploy/helm/dcm \
  --set k8sContainerServiceProvider.enabled=true \
  --set k8sContainerServiceProvider.namespace=default
```

OpenShift Routes are enabled by default for control-plane and DCM UI.

### Kubernetes

```bash
helm install dcm deploy/helm/dcm \
  --set controlPlane.route.enabled=false \
  --set dcmUi.route.enabled=false \
  --set k8sContainerServiceProvider.enabled=true \
  --set k8sContainerServiceProvider.namespace=default
```

Access via port-forward:

```bash
kubectl port-forward svc/dcm-control-plane 8080:8080
kubectl port-forward svc/dcm-dcm-ui 7007:7007
```

Then open:
- Control-plane API: http://localhost:8080
- DCM UI: http://localhost:7007

## Enabling Service Providers

### KubeVirt Service Provider

Manages virtual machines via KubeVirt.

```bash
helm upgrade dcm deploy/helm/dcm --reuse-values \
  --set kubevirtServiceProvider.enabled=true \
  --set kubevirtServiceProvider.namespace=default
```

### ACM Cluster Service Provider

Manages clusters via Red Hat Advanced Cluster Management.

**Pull secret** (required): Provide a base64-encoded `.dockerconfigjson` via one of:

- `pullSecret`: inline value (stored in a chart-managed Secret via `stringData`)
- `pullSecretRef`: name of a pre-existing Secret **in the release namespace** with a
  `stringData` key `pull-secret` whose value is the base64-encoded `.dockerconfigjson` string
  (`secretKeyRef` is same-namespace only)

> If both are set, `pullSecret` takes precedence and `pullSecretRef` is ignored.

```bash
# Encode your pull secret
PULL_SECRET=$(oc get secret pull-secret -n openshift-config -o jsonpath='{.data.\.dockerconfigjson}')
```

**Cluster access**: When both `kubeconfig` and `kubeconfigRef` are omitted, the chart creates
a ServiceAccount with RBAC for HyperShift, Hive, KubeVirt, Agent and core Secret APIs (in-cluster auth on the
hub). To use an external kubeconfig instead, provide it via one of:

- `kubeconfig`: raw kubeconfig content (stored in a chart-managed Secret)
- `kubeconfigRef`: name of a pre-existing Secret **in the release namespace** with key `kubeconfig`

```bash
# In-cluster mode (SA + RBAC created by chart):
helm upgrade dcm deploy/helm/dcm --reuse-values \
  --set acmClusterServiceProvider.enabled=true \
  --set acmClusterServiceProvider.namespace=default \
  --set acmClusterServiceProvider.baseDomain=example.com \
  --set acmClusterServiceProvider.pullSecret="$PULL_SECRET"

# External kubeconfig mode (inline):
helm upgrade dcm deploy/helm/dcm --reuse-values \
  --set acmClusterServiceProvider.enabled=true \
  --set acmClusterServiceProvider.namespace=default \
  --set acmClusterServiceProvider.baseDomain=example.com \
  --set acmClusterServiceProvider.pullSecret="$PULL_SECRET" \
  --set-file acmClusterServiceProvider.kubeconfig=/path/to/kubeconfig

# External kubeconfig mode (pre-existing Secret):
helm upgrade dcm deploy/helm/dcm --reuse-values \
  --set acmClusterServiceProvider.enabled=true \
  --set acmClusterServiceProvider.namespace=default \
  --set acmClusterServiceProvider.baseDomain=example.com \
  --set acmClusterServiceProvider.pullSecret="$PULL_SECRET" \
  --set acmClusterServiceProvider.kubeconfigRef=my-kubeconfig-secret
```

### Three-Tier Demo Service Provider

A demo provider for a three-tier application. Requires the Kubernetes Container Service Provider to also be enabled.

```bash
helm upgrade dcm deploy/helm/dcm --reuse-values \
  --set k8sContainerServiceProvider.enabled=true \
  --set threeTierDemoServiceProvider.enabled=true
```

## Authentication

Authentication mirrors the Compose stack: Keycloak as the IdP, and the control-plane
validates JWT bearer tokens (primary) or proxy headers (fallback). Route/Ingress
target the control-plane Service.

Auth is **disabled by default** (`auth.enabled=false`). Enabling it deploys Keycloak
and sets `AUTH_DISABLED=false` on the control-plane.

Default passwords and `proxySecret` are for **local/lab use only**. On shared
clusters, use `authSecretRef` or override inline credentials. Prefer simple
`devUserPassword` values (special characters can break realm import shell substitution).

**Credentials** (required when auth is enabled): provide via one of:

- Inline values (lab): chart renders a `{fullname}-auth` Secret from `auth.proxySecret`,
  `auth.keycloak.adminPassword`, and `auth.keycloak.devUserPassword`
- `authSecretRef`: name of a pre-existing Secret **in the release namespace** with keys
  `KEYCLOAK_ADMIN`, `KEYCLOAK_ADMIN_PASSWORD`, `DCM_DEV_USER_PASSWORD`, and
  `AUTH_PROXY_SECRET` (`secretKeyRef` is same-namespace only)

> When `authSecretRef` is set, inline defaults in `values.yaml` are ignored and the chart
> does not render an auth Secret. Rotate external Secrets with `kubectl rollout restart`
> on the Keycloak and control-plane Deployments.

### Enable authentication (lab)

```bash
helm upgrade dcm deploy/helm/dcm --reuse-values \
  --set auth.enabled=true
```

Or on install:

```bash
helm install dcm deploy/helm/dcm --set auth.enabled=true
```

**Shared cluster (pre-existing Secret):**

```bash
kubectl create secret generic dcm-auth \
  --from-literal=KEYCLOAK_ADMIN=admin \
  --from-literal=KEYCLOAK_ADMIN_PASSWORD='your-admin-password' \
  --from-literal=DCM_DEV_USER_PASSWORD='your-dev-password' \
  --from-literal=AUTH_PROXY_SECRET='your-proxy-secret'

helm install dcm deploy/helm/dcm \
  --set auth.enabled=true \
  --set auth.authSecretRef=dcm-auth
```

**Shared cluster (inline override):**

```bash
helm install dcm deploy/helm/dcm \
  --set auth.enabled=true \
  --set auth.proxySecret='your-proxy-secret' \
  --set auth.keycloak.adminPassword='your-admin-password' \
  --set auth.keycloak.devUserPassword='your-dev-password'
```

The realm source is `deploy/keycloak/realm-export.json` (same file Compose uses). The
chart bundles a copy under `files/`; after editing the source, run `make helm-chart-sync`
and commit both files. `make helm-chart-verify-sync` catches drift; CI runs
`make helm-chart-check` (verify, lint, template).

> **Warning:** Service providers do not forward authentication headers yet, so enabling
> auth can break SP workflows. The CLI (`dcm login` / bearer token) and direct API
> calls with a valid Keycloak JWT work.

### Auth values

| Value | Default | Description |
|---|---|---|
| `auth.enabled` | `false` | Deploy Keycloak and enable control-plane auth |
| `auth.authSecretRef` | _(empty)_ | Pre-existing Secret for auth credentials; when set, inline defaults ignored |
| `auth.proxySecret` | `dcm-dev-proxy-secret` | Shared secret for `X-Auth-Proxy-Secret` fallback (inline path only) |
| `auth.jwtAudience` | `dcm-api` | Expected JWT `aud`; empty skips audience check |
| `auth.issuerURL` | _(empty)_ | OIDC issuer; empty uses `http://<fullname>-keycloak:8080/realms/dcm` |
| `auth.keycloak.image` | `quay.io/keycloak/keycloak:26.0` | Keycloak image |
| `auth.keycloak.adminUser` / `adminPassword` | `admin` / `admin` | Keycloak admin console credentials |
| `auth.keycloak.devUserPassword` | `admin` | Password for the imported `dcm-admin` user |
| `auth.keycloak.route.enabled` | `false` | Optional OpenShift Route for the Keycloak admin console |
| `auth.keycloak.ingress.enabled` | `false` | Optional Ingress for the Keycloak admin console |

When exposing Keycloak via Route/Ingress, set `auth.issuerURL` to the external issuer
(for example `https://keycloak.apps.example.com/realms/dcm`) so token `iss` and
`KC_HOSTNAME` stay aligned.

### Access Keycloak

With the default ClusterIP Service, port-forward the Keycloak Service
(`<fullname>-keycloak`, e.g. `dcm-keycloak` for release `dcm`):

```bash
kubectl port-forward svc/dcm-keycloak 8180:8080
```

Then open http://localhost:8180 (admin / admin). The realm `dcm` includes clients
`dcm-proxy` (confidential, secret `dcm-proxy-secret`) and `dcm-cli`, and user
`dcm-admin` / `admin`.

### Call the API with a JWT

Lab-only password grant against the confidential `dcm-proxy` client. The password
grant is deprecated in OAuth 2.1; use it here only for local token acquisition,
not as a production pattern. Port-forward Keycloak and the control-plane first
(`svc/dcm-keycloak 8180:8080`, `svc/dcm-control-plane 8080:8080`):

```bash
TOKEN=$(curl -s -X POST 'http://localhost:8180/realms/dcm/protocol/openid-connect/token' \
  -d 'grant_type=password' \
  -d 'client_id=dcm-proxy' \
  -d 'client_secret=dcm-proxy-secret' \
  -d 'username=dcm-admin' \
  -d 'password=admin' | jq -r .access_token)

curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1alpha1/providers
```

`/api/v1alpha1/health` remains unauthenticated.

## Uninstall

```bash
helm uninstall dcm
```

Note: PersistentVolumeClaims for PostgreSQL and NATS are not deleted automatically. To remove them:

```bash
kubectl delete pvc -l app.kubernetes.io/instance=dcm
```
