# Running DCM

## Prerequisites

- [Podman](https://podman.io/) or [Docker](https://www.docker.com/) (the Makefile auto-detects which engine is available)
- (Optional) A Kubernetes cluster with KubeVirt for the kubevirt-service-provider
- (Optional) A Kubernetes cluster for the k8s-container-service-provider
- (Optional) An OpenShift cluster with ACM/MCE and HyperShift for the acm-cluster-service-provider

## Quick start

Start the core platform (postgres, nats, control-plane, and dcm-ui):

```bash
cp deploy/.env.example deploy/.env
make compose-up
```

`deploy/.env` holds database credentials and optional auth settings. Lab defaults are in
`deploy/.env.example`; copy and edit before first start.

`make compose-up` and `docker compose --env-file deploy/.env -f deploy/compose.yaml up` are
equivalent; the Makefile target is a thin wrapper around Compose.

The control-plane API is at `http://localhost:8080`. DCM UI is at `http://localhost:7007`.

Authentication is **disabled by default** (`AUTH_DISABLED=true`). Keycloak is behind the
`auth` compose profile and does not start with `make compose-up`. See
[Authentication](#authentication) for enabling it and current limitations.

`dcm login` requires Keycloak — use `make compose-up AUTH=true` after enabling auth in `.env`.

## CLI configuration

The [DCM CLI](https://github.com/dcm-project/cli) uses the same control-plane URL by default
(`http://localhost:8080`). Override it with the `control-plane-url` key in `~/.dcm/config.yaml`
or the `DCM_CONTROL_PLANE_URL` environment variable. See the [CLI README](https://github.com/dcm-project/cli/blob/main/README.md)
for install and usage.

The CLI forwards bearer tokens to the control-plane API. Run `dcm login` for interactive
OIDC device authorization (Keycloak `dcm-cli` client), or set `DCM_TOKEN` / `--token`
for CI and scripting.

## Running with service providers

Service providers are behind compose profiles and do not start by default.

### KubeVirt service provider

To include the `kubevirt-service-provider`, activate the `kubevirt` profile. Each provider
mounts a host kubeconfig at `/kubeconfig` (default `~/.kube/config`; override with
`KUBEVIRT_KUBECONFIG` in `deploy/.env` or the shell).

```bash
make compose-up-with-providers PROFILES=kubevirt
```

### K8s container service provider

To include the `k8s-container-service-provider`, activate the `k8s-container` profile:

```bash
make compose-up-with-providers PROFILES=k8s-container
```

If using Kind, see [K8s Container SP with Kind](docs/k8s-container-sp-kind.md) for additional network setup.

Optionally override the provider name or external service type:

```bash
export K8S_CONTAINER_SP_NAME=my-provider
export K8S_CONTAINER_SP_EXTERNAL_SVC_TYPE=LoadBalancer
```

### K8s storage service provider

To include the `k8s-storage-service-provider`, activate the `storage` profile:

```bash
make compose-up-with-providers PROFILES=storage
```

Optionally override the provider name, namespace, and default PVC behavior:

```bash
export K8S_STORAGE_SP_NAME=my-storage-provider
export K8S_STORAGE_SP_NAMESPACE=default
export K8S_STORAGE_SP_DEFAULT_STORAGE_CLASS=ceph-rbd
export K8S_STORAGE_SP_DEFAULT_ACCESS_MODE=ReadWriteOnce
```

### ACM cluster service provider

To include the `acm-cluster-service-provider`, set `ACM_CLUSTER_SP_PULL_SECRET` in
`deploy/.env` (base64-encoded `.dockerconfigjson`) and activate the `acm-cluster` profile:

```bash
# In deploy/.env:
# ACM_CLUSTER_SP_PULL_SECRET=<base64-encoded-dockerconfigjson>
make compose-up-with-providers PROFILES=acm-cluster
```

Optionally override the provider name, namespace, or base domain:

```bash
export ACM_CLUSTER_SP_NAME=my-acm-provider
export ACM_CLUSTER_SP_NAMESPACE=clusters
export ACM_CLUSTER_SP_BASE_DOMAIN="apps.example.com"
```

For BareMetal provisioning, also set:

```bash
export ACM_CLUSTER_SP_DEFAULT_INFRA_ENV="my-infra-env"
export ACM_CLUSTER_SP_AGENT_NAMESPACE="my-agent-namespace"
```

### Three-tier demo app service provider

To include the `three-tier-demo-service-provider`, activate the `three-tier` profile:

```bash
make compose-up-with-providers PROFILES=three-tier
```

When using Kind, complete the k8s-container setup (steps 1–5 in [K8s Container
SP with Kind](docs/k8s-container-sp-kind.md)) first.
For Pet Clinic usage, see [Three-Tier Demo App with Kind](docs/three-tier-app-kind.md).

Optionally override the provider name or cluster namespace (`K8S_CONTAINER_SP_NAMESPACE` applies
to both k8s-container and three-tier SPs):

```bash
export THREE_TIER_SP_NAME=my-provider
export K8S_CONTAINER_SP_NAMESPACE=default
```

### All providers

To start all providers at once, set `ACM_CLUSTER_SP_PULL_SECRET` in `deploy/.env` and run:

```bash
make compose-up-with-providers
```

This defaults to the `providers` Compose profile (all service providers, including the
three-tier demo SP). To start a single provider instead, pass `PROFILES=`:

```bash
make compose-up-with-providers PROFILES=kubevirt
make compose-up-with-providers PROFILES=k8s-container
make compose-up-with-providers PROFILES=storage
make compose-up-with-providers PROFILES=acm-cluster
make compose-up-with-providers PROFILES=three-tier
```

## Authentication

Keycloak (`:8180`) is the identity provider when the `auth` compose profile is active.
The control-plane validates JWT bearer tokens directly against Keycloak's JWKS endpoint
using OIDC discovery (no external auth proxy required). A proxy-header fallback path
(`X-Auth-Proxy-Secret` + `X-Forwarded-User`) is also supported.

Authentication is disabled by default (`AUTH_DISABLED=true`). When enabled, the CLI
(`dcm login` / bearer token) and direct JWT API calls work; service providers do not
forward authentication headers yet, so SP ↔ control-plane traffic may fail.

To enable authentication (Compose):

```bash
cp deploy/.env.example deploy/.env
# Uncomment the "Enable authentication" block in deploy/.env
make compose-up AUTH=true
```

Auth credentials live only in `deploy/.env` (see `deploy/.env.example`). Keycloak does
not start with `make compose-up`; pass `AUTH=true` when auth is enabled in `.env`.
With service providers: `make compose-up-with-providers PROFILES=kubevirt AUTH=true`.

For Helm chart installs, create the `dcm-auth` Secret and set `auth.enabled=true` — see
[helm/dcm/README.md](helm/dcm/README.md#authentication).

> **Warning:** Service providers do not forward authentication headers yet, so enabling
> auth can break SP workflows. The CLI (`dcm login` / bearer token) and direct API
> calls with a valid Keycloak JWT work.

When enabled, the control-plane authenticates requests via two paths (tried in order):

1. **JWT bearer token** (primary): `Authorization: Bearer <token>` — validated against Keycloak JWKS. Requires `AUTH_ISSUER_URL` to be set.
2. **Proxy headers** (fallback): `X-Auth-Proxy-Secret` + `X-Forwarded-User` — for callers routing through an auth proxy. Requires `AUTH_PROXY_SECRET` to be set.

The `/api/v1alpha1/health` endpoint is always unauthenticated.

Pre-configured lab credentials (set in `deploy/.env.example`):

| Service | URL | Username | Password |
|---|---|---|---|
| Keycloak admin console | `http://localhost:8180` | `admin` | `admin` (`KEYCLOAK_ADMIN_PASSWORD`) |
| DCM user (Keycloak) | — | `dcm-admin` | `admin` (`DCM_DEV_USER_PASSWORD`) |

The Keycloak realm is imported from `deploy/keycloak/realm-export.json` at container
start. The `dcm-admin` password is resolved from the `DCM_DEV_USER_PASSWORD` environment
variable via Keycloak's native import placeholders (`start-dev --import-realm`).
Prefer simple lab passwords; values with
`"`, `\`, or `$` may break native placeholder substitution. The realm includes
two clients: `dcm-proxy` (confidential, for service-to-service access) and `dcm-cli`
(public, for the DCM CLI device auth grant flow).

`DCM_ADMIN_SUBJECT` must match the `id` of a user in the Keycloak realm. The compose
default (`56deb662-4820-5d83-b828-f4beb11a5fa7`) corresponds to the pre-configured
`dcm-admin` user.

### Adding users

Create users in the Keycloak admin console at `http://localhost:8180` (login with
`admin` / `admin`). Navigate to the `dcm` realm, **Users → Add user**, fill in
a username, save, then set a password under the **Credentials** tab (disable
"Temporary"). Users must be in the `dcm` realm — the control-plane's OIDC
configuration points to this realm.

New users are automatically provisioned in the control-plane on first
authenticated request (JIT provisioning) — no manual DB setup is required.

## Verifying the deployment

Check that all services are running:

```bash
podman compose -f deploy/compose.yaml ps    # or: docker compose -f deploy/compose.yaml ps
```

Check the health endpoint (unauthenticated, works regardless of `AUTH_DISABLED`):

```bash
curl http://localhost:8080/api/v1alpha1/health
```

Check health endpoint through DCM UI:

```bash
curl http://localhost:7007/api/dcm/health
```

When authentication is enabled (`make compose-up AUTH=true`), verify Keycloak is ready:

```bash
podman compose -f deploy/compose.yaml --profile auth exec keycloak curl -sf http://localhost:9000/health/ready | jq .
```

## Stopping services

```bash
make compose-down
```

This stops all compose services and removes volumes. If Kind was connected to
the compose network (see [k8s-container-sp-kind.md](docs/k8s-container-sp-kind.md)),
`compose-down` disconnects external containers and removes both
`control-plane_default` and legacy `deploy_default` networks.

## Configuration

Database, auth, and ACM pull-secret credentials are defined in `deploy/.env.example`
(copy to `deploy/.env`). The table below lists non-secret knobs and provider settings.

| Variable                                   | Default                     | Description                                                                                                 |
| ------------------------------------------ | --------------------------- | ----------------------------------------------------------------------------------------------------------- |
| `AUTH_DISABLED`                             | `true`                      | Disable authentication (see [Authentication](#authentication); set in `.env`)                                 |
| `AUTH_ISSUER_URL`                           | _(empty)_                   | OIDC issuer URL for JWT validation (e.g. `http://keycloak:8080/realms/dcm`)                                 |
| `AUTH_JWT_AUDIENCE`                         | `dcm-api`                   | Expected `aud` claim in JWT tokens                                                                            |
| `AUTH_PROXY_SECRET`                         | _(in `.env.example`)        | Shared secret for proxy-header fallback auth path                                                           |
| `AUTH_CACHE_TTL`                            | `60s`                       | TTL for the actor resolution cache                                                                          |
| `DCM_ADMIN_SUBJECT`                        | `56deb662-...`              | Keycloak subject UUID for the bootstrap admin actor (required when auth enabled)                            |
| `POSTGRES_USER` / `POSTGRES_PASSWORD`      | _(in `.env.example`)        | PostgreSQL credentials (also `DB_USER`, `DB_PASS`, `DB_PASSWORD`)                                           |
| `KEYCLOAK_ADMIN_PASSWORD`                  | _(in `.env.example`)        | Keycloak admin console password                                                                             |
| `DCM_DEV_USER_PASSWORD`                     | _(in `.env.example`)        | Password for the `dcm-admin` dev user in Keycloak                                                           |
| `KUBERNETES_NAMESPACE`                     | `default`                   | Kubernetes namespace for KubeVirt VMs                                                                       |
| `KUBEVIRT_KUBECONFIG`                      | `~/.kube/config`            | Host path to kubeconfig for the kubevirt-service-provider                                                   |
| `KUBEVIRT_PROVIDER_NAME`                   | `kubevirt-service-provider` | Provider name and Compose service `container_name`                                                          |
| `K8S_CONTAINER_SP_KUBECONFIG`              | `~/.kube/config`            | Host path to kubeconfig for the k8s-container and three-tier service providers                            |
| `K8S_CONTAINER_SP_NAMESPACE`               | `default`                   | Kubernetes namespace for k8s containers                                                                     |
| `K8S_CONTAINER_SP_NAME`                    | `k8s-container-provider`    | Provider name for the k8s-container-service-provider                                                        |
| `K8S_CONTAINER_SP_EXTERNAL_SVC_TYPE`       | `NodePort`                  | Kubernetes Service type for external ports (`NodePort` or `LoadBalancer`)                                   |
| `K8S_STORAGE_SP_KUBECONFIG`                | `~/.kube/config`            | Host path to kubeconfig for the k8s-storage-service-provider                                                |
| `K8S_STORAGE_SP_NAMESPACE`                 | `default`                   | Kubernetes namespace used by the k8s-storage-service-provider                                               |
| `K8S_STORAGE_SP_NAME`                      | `k8s-storage-provider`      | Provider name for the k8s-storage-service-provider                                                          |
| `K8S_STORAGE_SP_DEFAULT_STORAGE_CLASS`     | _(empty)_                   | Optional fallback StorageClass when request hints do not set one                                            |
| `K8S_STORAGE_SP_DEFAULT_ACCESS_MODE`       | `ReadWriteOnce`             | Optional fallback access mode when request hints do not set one                                             |
| `ACM_CLUSTER_SP_KUBECONFIG`                | `~/.kube/config`            | Host path to kubeconfig for the acm-cluster-service-provider                                                |
| `ACM_CLUSTER_SP_NAMESPACE`                 | `default`                   | Kubernetes namespace for ACM hosted clusters                                                                |
| `ACM_CLUSTER_SP_NAME`                      | `acm-cluster-sp`            | Provider name for the acm-cluster-service-provider                                                          |
| `ACM_CLUSTER_SP_BASE_DOMAIN`               | _(none)_                    | Base DNS domain for hosted clusters; can be overridden per-request via `provider_hints.acm.base_domain`     |
| `ACM_CLUSTER_SP_PULL_SECRET`               | _(in `.env`)                | Base64-encoded dockerconfigjson pull secret for ACM hosted clusters (required for `acm-cluster` profile)    |
| `ACM_CLUSTER_SP_DEFAULT_INFRA_ENV`         | _(none)_                    | **BareMetal only.** Default InfraEnv name; can be overridden per-request via `provider_hints.acm.infra_env` |
| `ACM_CLUSTER_SP_AGENT_NAMESPACE`           | _(none)_                    | **BareMetal only.** Namespace where Agent resources are located                                             |
| `CONTROL_PLANE_VERSION`                    | `main`                      | Image tag for control-plane monolith                                                                        |
| `KUBEVIRT_SERVICE_PROVIDER_VERSION`        | `main`                      | Image tag for kubevirt-service-provider                                                                     |
| `K8S_CONTAINER_SERVICE_PROVIDER_VERSION`   | `main`                      | Image tag for k8s-container-service-provider                                                                |
| `K8S_STORAGE_SERVICE_PROVIDER_VERSION`     | `main`                      | Image tag for k8s-storage-service-provider                                                                  |
| `ACM_CLUSTER_SERVICE_PROVIDER_VERSION`     | `main`                      | Image tag for acm-cluster-service-provider                                                                  |
| `THREE_TIER_DEMO_SERVICE_PROVIDER_VERSION` | `main`                      | Image tag for three-tier-demo-service-provider                                                              |
| `THREE_TIER_SP_NAME`                       | `three-tier-provider`       | Provider name for the three-tier-demo-service-provider                                                      |
| `DCM_UI_VERSION`                           | `main`                      | Image tag for dcm-ui                                                                                        |

See [Image versions](../README.md#image-versions) in the README for available tag formats and how to update.

## Kubernetes / OpenShift

See [helm/dcm/README.md](helm/dcm/README.md). Create Kubernetes Secrets before install
(`dcm-db` always; `dcm-auth` when `auth.enabled=true`; `dcm-acm-pull-secret` when ACM SP
is enabled). Lab `kubectl create secret` examples are in the Helm README.
