# Running DCM

## Prerequisites

- [Podman](https://podman.io/) or [Docker](https://www.docker.com/) (the Makefile auto-detects which engine is available)
- (Optional) [Kind](https://kind.sigs.k8s.io/) with KubeVirt for the environment-agent embedded `vm` SP
- (Optional) A Kubernetes cluster for environment-agent embedded `container` and `cluster` SPs
- (Optional) [utilities](https://github.com/dcm-project/utilities) repo as a sibling directory for Kind helper scripts (`../utilities`)

## Quick start

### With environment-agent (Service Providers)

An [environment-agent](https://github.com/dcm-project/environment-agent) must be running and
registered with the control-plane in order to use Service Providers. Follow the guide,
[environment-agent-kind.md](docs/environment-agent-kind.md) for the full setup.

### Control-plane and UI only

If you only need the API and UI (no workload provisioning):

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

## Running with Environment Agent

See [environment-agent-kind.md](docs/environment-agent-kind.md) for Kind setup, the
`environment-agent` compose profile, configuration, and verification.

## Authentication

Keycloak (`:8180`) is the identity provider when the `auth` compose profile is active.
The control-plane validates JWT bearer tokens directly against Keycloak's JWKS endpoint
using OIDC discovery (no external auth proxy required). A proxy-header fallback path
(`X-Auth-Proxy-Secret` + `X-Forwarded-User`) is also supported.

Authentication is disabled by default (`AUTH_DISABLED=true`). When enabled, the CLI
(`dcm login` / bearer token) and direct JWT API calls work; the environment-agent does not
forward authentication headers yet, so SP workflows may fail.

To enable authentication (Compose):

```bash
cp deploy/.env.example deploy/.env
# Uncomment the "Enable authentication" block in deploy/.env
make compose-up AUTH=true
```

Auth credentials live only in `deploy/.env` (see `deploy/.env.example`). Keycloak does
not start with `make compose-up`; pass `AUTH=true` when auth is enabled in `.env`.
With the environment-agent: `make compose-up-with-agent AUTH=true`.

For Helm chart installs, create the `dcm-auth` Secret and set `auth.enabled=true` — see
[helm/dcm/README.md](helm/dcm/README.md#authentication).

> **Warning:** The environment-agent does not forward authentication headers yet, so enabling
> auth can break SP workflows. The CLI (`dcm login` / bearer token) and direct API calls with a
> valid Keycloak JWT work.

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
the compose network (see [environment-agent-kind.md](docs/environment-agent-kind.md)),
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
| `AGENT_NAME`                               | `local-agent`               | Agent name for environment-agent profile                                                                    |
| `AGENT_ENVIRONMENT`                        | `dev`                       | Environment classification for environment-agent                                                            |
| `AGENT_COST`                               | `low`                       | Cost classification for environment-agent                                                                   |
| `AGENT_PORT`                               | `8081`                      | Host port for environment-agent HTTP API                                                                    |
| `AGENT_EMBEDDED_SPS`                       | _(empty)_                   | **Required in `deploy/.env`** when using the agent profile. Comma-separated: `container`, `vm`, `cluster`, `storage` |
| `AGENT_KUBECONFIG_HOST`                    | `~/.kube/config`            | Host kubeconfig bind mount; use `.kube/config` in `deploy/.env` with Kind (`make kubeconfig-for-compose`) |
| `SP_DEFAULT_KUBECONFIG`                    | `/kubeconfig`               | In-container kubeconfig path for embedded SPs (set in `compose.yaml`; do not set in `.env`)               |
| `SP_CONTAINER_NAMESPACE`                   | `default`                   | Container SP workload namespace (environment-agent)                                                         |
| `SP_K8S_EXTERNAL_SVC_TYPE`                 | `NodePort`                  | Container SP external service type (environment-agent)                                                      |
| `SP_VM_NAMESPACE`                          | `default`                   | VM SP workload namespace (environment-agent)                                                                  |
| `SP_CLUSTER_NAMESPACE`                     | _(required for cluster SP)_ | ACM cluster namespace (environment-agent cluster SP)                                                        |
| `SP_PULL_SECRET`                           | _(required for cluster SP)_ | Base64-encoded dockerconfigjson for environment-agent cluster SP                                            |
| `SP_BASE_DOMAIN`                           | _(none)_                    | Base domain for hosted clusters (environment-agent cluster SP)                                              |
| `SP_STORAGE_NAMESPACE`                     | `default`                   | Storage SP workload namespace (environment-agent)                                                           |
| `SP_K8S_DEFAULT_STORAGE_CLASS`             | _(none)_                    | Default storage class for environment-agent storage SP                                                      |
| `SP_K8S_DEFAULT_ACCESS_MODE`               | `ReadWriteOnce`             | Default PVC access mode for environment-agent storage SP                                                    |
| `ENVIRONMENT_AGENT_VERSION`                | `main`                      | Image tag for environment-agent                                                                             |
| `CONTROL_PLANE_VERSION`                    | `main`                      | Image tag for control-plane monolith                                                                        |
| `DCM_UI_VERSION`                           | `main`                      | Image tag for dcm-ui                                                                                        |

See [Image versions](../README.md#image-versions) in the README for available tag formats and how to update.

## Kubernetes / OpenShift

See [helm/dcm/README.md](helm/dcm/README.md). Create Kubernetes Secrets before install
(`dcm-db` always; `dcm-auth` when `auth.enabled=true`; `dcm-acm-pull-secret` when ACM SP
is enabled). Lab `kubectl create secret` examples are in the Helm README.
