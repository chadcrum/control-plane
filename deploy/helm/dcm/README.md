# DCM Helm Chart

AI agents maintaining this chart: see [AGENTS.md](AGENTS.md).

## Prerequisites

- Kubernetes 1.24+ or OpenShift 4.12+
- Helm 3.x
- A default StorageClass configured in the cluster (for PostgreSQL and NATS persistent volumes)

## Quick Start

Create the database Secret in the target namespace before install (lab defaults):

```bash
kubectl create secret generic dcm-db \
  --from-literal=POSTGRES_USER=admin \
  --from-literal=POSTGRES_PASSWORD=adminpass \
  --from-literal=DB_USER=admin \
  --from-literal=DB_PASS=adminpass \
  --from-literal=DB_PASSWORD=adminpass
```

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

**Pull secret** (required when enabled): create a pre-existing Secret **in the release
namespace** with a `stringData` key `pull-secret` whose value is the base64-encoded
`.dockerconfigjson` string, then set `acmClusterServiceProvider.pullSecretRef` (default
`dcm-acm-pull-secret`).

```bash
PULL_SECRET=$(oc get secret pull-secret -n openshift-config -o jsonpath='{.data.\.dockerconfigjson}')
kubectl create secret generic dcm-acm-pull-secret \
  --from-literal=pull-secret="$PULL_SECRET"
```

**Cluster access**: When `kubeconfigRef` is omitted, the chart creates a ServiceAccount
with RBAC for HyperShift, Hive, KubeVirt, Agent and core Secret APIs (in-cluster auth on the
hub). To use an external kubeconfig, create a Secret with key `kubeconfig` and set
`acmClusterServiceProvider.kubeconfigRef`.

```bash
# In-cluster mode (SA + RBAC created by chart):
helm upgrade dcm deploy/helm/dcm --reuse-values \
  --set acmClusterServiceProvider.enabled=true \
  --set acmClusterServiceProvider.namespace=default \
  --set acmClusterServiceProvider.baseDomain=example.com

# External kubeconfig mode (pre-existing Secret):
kubectl create secret generic my-kubeconfig-secret \
  --from-file=kubeconfig=/path/to/kubeconfig
helm upgrade dcm deploy/helm/dcm --reuse-values \
  --set acmClusterServiceProvider.enabled=true \
  --set acmClusterServiceProvider.namespace=default \
  --set acmClusterServiceProvider.baseDomain=example.com \
  --set acmClusterServiceProvider.kubeconfigRef=my-kubeconfig-secret
```

### Three-Tier Demo Service Provider

A demo provider for a three-tier application. Requires the Kubernetes Container Service Provider to also be enabled.

```bash
helm upgrade dcm deploy/helm/dcm --reuse-values \
  --set k8sContainerServiceProvider.enabled=true \
  --set threeTierDemoServiceProvider.enabled=true
```

## Values schema and maintainability

**Install-time validation**: `deploy/helm/dcm/values.schema.json` is the contract that helm validates against at install/upgrade/lint time.

**Keep in sync:** When modifying `values.yaml`, also update `values.schema.json` to catch mistypes or unknown values early. Running `make helm-chart-sync` then `make helm-chart-check` ensures your changes don't introduce lint errors.

**Template-time rules**: `scripts/verify-template.sh` remains authoritative for render-time constraints that the schema cannot express (e.g., "Secret must not emit when authSecretRef is set"). Schema guards and template guards operate independently.

### Run full validation

```bash
make helm-chart-sync   # realm file
make helm-chart-check  # verify/schema/lint/template
scripts/verify-template.sh deploy/helm/dcm    # positive & negative tests
```

## Authentication

Authentication mirrors the Compose stack: Keycloak as the IdP, and the control-plane
validates JWT bearer tokens (primary) or proxy headers (fallback). Route/Ingress
target the control-plane Service.

Auth is **disabled by default** (`auth.enabled=false`). Enabling it deploys Keycloak
and sets `AUTH_DISABLED=false` on the control-plane.

**Credentials** (required when auth is enabled): create a pre-existing Secret **in the
release namespace** with keys `KEYCLOAK_ADMIN`, `KEYCLOAK_ADMIN_PASSWORD`,
`DCM_DEV_USER_PASSWORD`, and `AUTH_PROXY_SECRET`, then set `auth.authSecretRef` (default
`dcm-auth`). The chart does not render credential Secrets.

> Rotate external Secrets with `kubectl rollout restart` on the Keycloak and
> control-plane Deployments.

### Enable authentication (lab)

```bash
kubectl create secret generic dcm-auth \
  --from-literal=KEYCLOAK_ADMIN=admin \
  --from-literal=KEYCLOAK_ADMIN_PASSWORD=admin \
  --from-literal=DCM_DEV_USER_PASSWORD=admin \
  --from-literal=AUTH_PROXY_SECRET=dcm-dev-proxy-secret

helm upgrade dcm deploy/helm/dcm --reuse-values \
  --set auth.enabled=true
```

Or on install:

```bash
helm install dcm deploy/helm/dcm \
  --set auth.enabled=true
```

(`auth.authSecretRef` defaults to `dcm-auth`; create the Secret in the release namespace first.)

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
| `auth.authSecretRef` | `dcm-auth` | Pre-existing Secret for auth credentials (required when auth enabled) |
| `auth.jwtAudience` | `dcm-api` | Expected JWT `aud`; empty skips audience check |
| `auth.issuerURL` | _(empty)_ | OIDC issuer; empty uses `http://<fullname>-keycloak:8080/realms/dcm` |
| `auth.keycloak.image` | `quay.io/keycloak/keycloak:26.0.1` | Keycloak image |
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
`dcm-proxy` (confidential; secret from `AUTH_PROXY_SECRET` in `dcm-auth`) and `dcm-cli`,
and user `dcm-admin` / `admin`.

### Call the API with a JWT

Lab-only password grant against the confidential `dcm-proxy` client. The password
grant is deprecated in OAuth 2.1; use it here only for local token acquisition,
not as a production pattern. Port-forward Keycloak and the control-plane first
(`svc/dcm-keycloak 8180:8080`, `svc/dcm-control-plane 8080:8080`):

```bash
TOKEN=$(curl -s -X POST 'http://localhost:8180/realms/dcm/protocol/openid-connect/token' \
  -d 'grant_type=password' \
  -d 'client_id=dcm-proxy' \
  -d 'client_secret=dcm-dev-proxy-secret' \
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
