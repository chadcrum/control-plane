# control-plane

Monorepo for the DCM control plane monolith.

## Layout

```text
cmd/control-plane/              # monolith entrypoint
internal/app/                   # wiring, config, shared DB bootstrap
internal/catalog/               # catalog manager
internal/placement/             # placement manager
internal/policy/                # policy manager (CRUD + OPA engine)
internal/sp/                    # service provider manager
api/                            # OpenAPI specs per domain
pkg/                            # generated HTTP clients
test/subsystem/                 # per-domain subsystem tests
deploy/                         # platform compose, helm chart, postgres init
```

See the [control plane monolith enhancement](https://github.com/dcm-project/enhancements/tree/main/enhancements/control-plane-monolith).

## Development

Build and unit tests:

```bash
make build
make test
make lint
make check-catalog-aep
```

Run the monolith (pick one):

| Command | Where it runs | Database | Other deps |
|---------|---------------|----------|------------|
| `make run` | host | SQLite at `/tmp/control-plane.db` | NATS disabled |
| `make run-dev` | host | Postgres (`DB_*` defaults) | Postgres + NATS running locally |
| `make compose-up` | containers | Postgres in compose | also starts NATS, control-plane, and dcm-ui (no Keycloak) |

```bash
make run              # SQLite, no containers
cp deploy/.env.example deploy/.env
make compose-up       # platform stack in containers
make compose-up AUTH=true  # same + Keycloak when auth is enabled in .env
make compose-down     # stop stack and remove volumes
```

Compose credentials live in `deploy/.env` (copy from `deploy/.env.example`). Keycloak
uses the `auth` compose profile — `make compose-up AUTH=true` after uncommenting the auth block
in `.env`. With providers: `make compose-up-with-providers PROFILES=kubevirt AUTH=true`.
Subsystem tests use the shared `test/subsystem/.env.example` file.

Policy evaluation and placement provisioning run in-process in the monolith
(`EvaluationService`, `PlacementService` via local clients). There is no public
HTTP route for `policies:evaluateRequest` or `/resources`. Use
`make test-policy` and `make test-placement` for unit coverage.

Per-domain subsystem tests still run separately; catalog compose may set
`PLACEMENT_MANAGER_URL` to reach WireMock placement stubs (test-only; not a
production API on the monolith).

Legacy `cmd/*-manager` binaries were removed. Use `make build` / `make run`.

### Container image

Build locally:

```bash
make image-build
```

CI pushes to `quay.io/dcm-project/control-plane` on merges to `main` and
`release/v*` branches (and on version tags). See
[Releasing](https://github.com/dcm-project/shared-workflows#release-flow)
in shared-workflows for tag behavior and version conventions.

## Platform deploy

Full-stack Compose and Helm packaging live under `deploy/`:

- **Compose:** control-plane, postgres, nats, keycloak, dcm-ui, and optional service providers
- **Helm:** Kubernetes/OpenShift chart at `deploy/helm/dcm` (optional auth via `auth.enabled`)

See [deploy/RUN.md](deploy/RUN.md) for local stack usage, authentication, and service provider profiles.
See [deploy/helm/dcm/README.md](deploy/helm/dcm/README.md) for cluster installs.

Authentication is disabled by default (`AUTH_DISABLED=true`). The CLI forwards
bearer tokens (`dcm login` or `DCM_TOKEN`); service providers do not yet forward
authentication headers, so enabling auth can break SP communication with the
control plane.

| Port | Service | Notes |
|---|---|---|
| `:8080` | control-plane API | Direct access; validates JWT bearer tokens when auth enabled |
| `:8180` | Keycloak | Identity provider (OIDC issuer) |
| `:7007` | dcm-ui | Web UI |

### Image versions

Service repos push images to `quay.io/dcm-project/<service>` with the following tags:

| Tag format | Example | When created |
|---|---|---|
| `main` | `main` | Every push to `main` |
| `<short-sha>` | `a6882f7` | Every push to `main` or `release/v*` branch, and `v*` git tag pushes |
| `v<semver>-rc.N` | `v0.0.1-rc.2` | Retag script (promotes a release branch SHA to an RC tag) |
| `v<semver>` | `v0.0.1` | When a `v*` git tag is pushed (final release) |

Browse available tags at `https://quay.io/repository/dcm-project/<service>?tab=tags`.

Pin versions in `.env` (see `deploy/.env.example`):

```bash
CONTROL_PLANE_VERSION=v0.0.1
```

Omitting the variable defaults to `main`.
