# Running DCM

## Prerequisites

- [Podman](https://podman.io/) or [Docker](https://www.docker.com/) (the Makefile auto-detects which engine is available)
- (Optional) A Kubernetes cluster with KubeVirt for the kubevirt-service-provider
- (Optional) A Kubernetes cluster for the k8s-container-service-provider
- (Optional) An OpenShift cluster with ACM/MCE and HyperShift for the acm-cluster-service-provider

## Quick start

Start the core platform (postgres, nats, keycloak, control-plane, and dcm-ui):

```bash
make compose-up
```

The control-plane API is at `http://localhost:8080`. DCM UI is at `http://localhost:7007`.

Authentication is **disabled by default** (`AUTH_DISABLED=true`). See
[Authentication](#authentication) for enabling it and current limitations.

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

To include the `kubevirt-service-provider`, set the required environment variables and
activate the `kubevirt` profile:

```bash
export KUBERNETES_NAMESPACE=vms
export KUBEVIRT_KUBECONFIG="/path/to/kubeconfig"
make compose-up-with-providers PROFILES=kubevirt
```

### K8s container service provider

To include the `k8s-container-service-provider`, set the required environment variables and
activate the `k8s-container` profile:

```bash
export K8S_CONTAINER_SP_KUBECONFIG="/path/to/kubeconfig"
make compose-up-with-providers PROFILES=k8s-container
```

If using Kind, see [K8s Container SP with Kind](docs/k8s-container-sp-kind.md) for additional network setup.

Optionally override the provider name or external service type:

```bash
export K8S_CONTAINER_SP_NAME=my-provider
export K8S_CONTAINER_SP_EXTERNAL_SVC_TYPE=LoadBalancer
```

### ACM cluster service provider

To include the `acm-cluster-service-provider`, set the required environment variables and
activate the `acm-cluster` profile:

```bash
export ACM_CLUSTER_SP_KUBECONFIG="/path/to/kubeconfig"
export ACM_CLUSTER_SP_PULL_SECRET="<base64-encoded-dockerconfigjson>"
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

To include the `three-tier-demo-service-provider`, set the required environment variables and
activate the `three-tier` profile:

```bash
export K8S_CONTAINER_SP_KUBECONFIG="/path/to/kubeconfig"
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

To start all providers at once, set the required environment variables and run:

```bash
export KUBEVIRT_KUBECONFIG="/path/to/kubeconfig"
export K8S_CONTAINER_SP_KUBECONFIG="/path/to/kubeconfig"
export ACM_CLUSTER_SP_KUBECONFIG="/path/to/kubeconfig"
export ACM_CLUSTER_SP_PULL_SECRET="<base64-encoded-dockerconfigjson>"
# BareMetal only:
export ACM_CLUSTER_SP_DEFAULT_INFRA_ENV="my-infra-env"
export ACM_CLUSTER_SP_AGENT_NAMESPACE="my-agent-namespace"
make compose-up-with-providers
```

This defaults to the `providers` Compose profile (all service providers, including the
three-tier demo SP). To start a single provider instead, pass `PROFILES=`:

```bash
make compose-up-with-providers PROFILES=kubevirt
make compose-up-with-providers PROFILES=k8s-container
make compose-up-with-providers PROFILES=acm-cluster
make compose-up-with-providers PROFILES=three-tier
```

## Authentication

The compose stack includes [Keycloak](https://www.keycloak.org/) (`:8180`) as the identity
provider. The control-plane validates JWT bearer tokens directly against Keycloak's
JWKS endpoint using OIDC discovery (no external auth proxy required). A proxy-header
fallback path (`X-Auth-Proxy-Secret` + `X-Forwarded-User`) is also supported.

Authentication is disabled by default (`AUTH_DISABLED=true`). When enabled, the CLI
(`dcm login` / bearer token) and direct JWT API calls work; service providers do not
forward authentication headers yet, so SP ↔ control-plane traffic may fail.

To enable authentication (Compose):

```bash
AUTH_DISABLED=false AUTH_ISSUER_URL=http://keycloak:8080/realms/dcm make compose-up
```

For Helm chart installs, see [helm/dcm/README.md](helm/dcm/README.md#authentication)
(`auth.enabled=true`).

> **Warning:** Service providers do not forward authentication headers yet, so enabling
> auth can break SP workflows. The CLI (`dcm login` / bearer token) and direct API
> calls with a valid Keycloak JWT work.

When enabled, the control-plane authenticates requests via two paths (tried in order):

1. **JWT bearer token** (primary): `Authorization: Bearer <token>` — validated against Keycloak JWKS. Requires `AUTH_ISSUER_URL` to be set.
2. **Proxy headers** (fallback): `X-Auth-Proxy-Secret` + `X-Forwarded-User` — for callers routing through an auth proxy. Requires `AUTH_PROXY_SECRET` to be set.

The `/api/v1alpha1/health` endpoint is always unauthenticated.

Pre-configured credentials (local dev only):

| Service | URL | Username | Password |
|---|---|---|---|
| Keycloak admin console | `http://localhost:8180` | `admin` | `admin` |
| DCM user (Keycloak) | — | `dcm-admin` | `admin` |

The Keycloak realm is imported from `deploy/keycloak/realm-export.json` and includes
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

When authentication is enabled, verify Keycloak is ready:

```bash
podman compose -f deploy/compose.yaml exec keycloak curl -sf http://localhost:9000/health/ready | jq .
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

| Variable                                   | Default                     | Description                                                                                                 |
| ------------------------------------------ | --------------------------- | ----------------------------------------------------------------------------------------------------------- |
| `AUTH_DISABLED`                             | `true`                      | Disable authentication (default `true`; see [Authentication](#authentication))                              |
| `AUTH_ISSUER_URL`                           | _(empty)_                   | OIDC issuer URL for JWT validation (e.g. `http://keycloak:8080/realms/dcm`). Empty = JWT path disabled.     |
| `AUTH_JWT_AUDIENCE`                         | _(empty)_                   | Expected `aud` claim in JWT tokens. Empty = audience check skipped.                                         |
| `AUTH_PROXY_SECRET`                         | `dcm-dev-proxy-secret`      | Shared secret for proxy-header fallback auth path                                                           |
| `AUTH_CACHE_TTL`                            | `60s`                       | TTL for the actor resolution cache                                                                          |
| `DCM_ADMIN_SUBJECT`                        | `56deb662-...` _(see below)_ | Keycloak subject UUID for the bootstrap admin actor (required when auth enabled)                            |
| `KEYCLOAK_ADMIN_PASSWORD`                  | `admin`                     | Keycloak admin console password                                                                             |
| `DCM_DEV_USER_PASSWORD`                     | `admin`                     | Password for the `dcm-admin` dev user in Keycloak                                                           |
| `POSTGRES_USER`                            | `admin`                     | PostgreSQL username                                                                                         |
| `POSTGRES_PASSWORD`                        | `adminpass`                 | PostgreSQL password                                                                                         |
| `KUBERNETES_NAMESPACE`                     | `default`                   | Kubernetes namespace for KubeVirt VMs                                                                       |
| `KUBEVIRT_KUBECONFIG`                      | `~/.kube/config`            | Path to kubeconfig on the host                                                                              |
| `KUBEVIRT_PROVIDER_NAME`                   | `kubevirt-service-provider` | Provider name and Compose service `container_name`                                                          |
| `K8S_CONTAINER_SP_KUBECONFIG`              | `~/.kube/config`            | Path to kubeconfig on the host for the k8s-container-service-provider                                       |
| `K8S_CONTAINER_SP_NAMESPACE`               | `default`                   | Kubernetes namespace for k8s containers                                                                     |
| `K8S_CONTAINER_SP_NAME`                    | `k8s-container-provider`    | Provider name for the k8s-container-service-provider                                                        |
| `K8S_CONTAINER_SP_EXTERNAL_SVC_TYPE`       | `NodePort`                  | Kubernetes Service type for external ports (`NodePort` or `LoadBalancer`)                                   |
| `ACM_CLUSTER_SP_KUBECONFIG`                | `~/.kube/config`            | Path to kubeconfig on the host for the acm-cluster-service-provider                                         |
| `ACM_CLUSTER_SP_NAMESPACE`                 | `default`                   | Kubernetes namespace for ACM hosted clusters                                                                |
| `ACM_CLUSTER_SP_NAME`                      | `acm-cluster-sp`            | Provider name for the acm-cluster-service-provider                                                          |
| `ACM_CLUSTER_SP_BASE_DOMAIN`               | _(none)_                    | Base DNS domain for hosted clusters; can be overridden per-request via `provider_hints.acm.base_domain`     |
| `ACM_CLUSTER_SP_PULL_SECRET`               | _(required)_                | Base64-encoded dockerconfigjson pull secret for ACM hosted clusters                                         |
| `ACM_CLUSTER_SP_DEFAULT_INFRA_ENV`         | _(none)_                    | **BareMetal only.** Default InfraEnv name; can be overridden per-request via `provider_hints.acm.infra_env` |
| `ACM_CLUSTER_SP_AGENT_NAMESPACE`           | _(none)_                    | **BareMetal only.** Namespace where Agent resources are located                                             |
| `CONTROL_PLANE_VERSION`                    | `main`                      | Image tag for control-plane monolith                                                                        |
| `KUBEVIRT_SERVICE_PROVIDER_VERSION`        | `main`                      | Image tag for kubevirt-service-provider                                                                     |
| `K8S_CONTAINER_SERVICE_PROVIDER_VERSION`   | `main`                      | Image tag for k8s-container-service-provider                                                                |
| `ACM_CLUSTER_SERVICE_PROVIDER_VERSION`     | `main`                      | Image tag for acm-cluster-service-provider                                                                  |
| `THREE_TIER_DEMO_SERVICE_PROVIDER_VERSION` | `main`                      | Image tag for three-tier-demo-service-provider                                                              |
| `THREE_TIER_SP_NAME`                       | `three-tier-provider`       | Provider name for the three-tier-demo-service-provider                                                      |
| `DCM_UI_VERSION`                           | `main`                      | Image tag for dcm-ui                                                                                        |

See [Image versions](../README.md#image-versions) in the README for available tag formats and how to update.

## Kubernetes / OpenShift

See [helm/dcm/README.md](helm/dcm/README.md).
