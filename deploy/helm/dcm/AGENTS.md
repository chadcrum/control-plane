# AGENTS.md - DCM Helm chart

Guidance for AI agents editing `deploy/helm/dcm/`. Humans installing or operating the chart should read [README.md](README.md).

## Scope

This chart deploys the DCM control plane stack (PostgreSQL, NATS, control-plane, optional UI, optional service providers, optional Keycloak auth). Chart path: `deploy/helm/dcm/`.

## Source of truth

| File | Role |
|------|------|
| `values.yaml` | Default values and chart structure |
| `values.schema.json` | Install-time contract (`helm lint` / install / upgrade) |
| `templates/` | Rendered Kubernetes manifests; template `fail` for render-time rules |
| `scripts/verify-template.sh` | Positive and negative render-time tests |
| `scripts/verify-schema.sh` | Negative schema validation tests |
| `README.md` | User-facing install, auth, and service-provider docs |

Do not auto-generate `values.schema.json`. Hand-maintain it to mirror `values.yaml`.

## Layered documentation

Each fact belongs in one primary place. Do not duplicate README prose into YAML or schema.

| Layer | Put here |
|-------|----------|
| Types, defaults, `if`/`then` rules | `values.schema.json` (`description` on non-obvious fields) |
| Defaults, `# --` section headers, short inline hints | `values.yaml` |
| How-to guides and examples | `README.md` |
| Render-time conditionals schema cannot express | Templates and `verify-template.sh` |

When removing an inline comment, add an equivalent `description` in the schema or keep the comment. Do not drop documentation.

## When changing values

1. Edit the default in `values.yaml`.
2. Mirror structure, types, and defaults in `values.schema.json`. Add or update `description` when behavior is not obvious (empty string semantics, secret refs, auth conditionals).
3. Update `README.md` only when user-facing behavior or examples change.
4. If render-time behavior changes, update templates and `scripts/verify-template.sh`.
5. If a new schema negative case matters, add it to `scripts/verify-schema.sh`.
6. Run verification (see below).

### `values.yaml` comment style

- Keep the top-line schema sync comment.
- Use `# -- Section name` for top-level blocks.
- Use end-of-line comments only for short, edit-time hints (for example `tag: ""  # defaults to global.imageTag`).
- Do not add multi-paragraph comment blocks; put that detail in README.

### Schema rules

- `additionalProperties: false` on every object.
- Do not add keys that are absent from `values.yaml` (no `nameOverride`, `nexus3`, etc.).
- Auth conditionals live in the `auth` object `allOf` block.
- Reuse `$defs` for repeated shapes (`route`, `ingress`, `securityContext`, `keycloakConfig`).

### Realm sync

`auth.adminSubject` must match the `dcm-admin` user id in `deploy/keycloak/realm-export.json`. After changing either file, run `make helm-chart-sync` and `make helm-chart-verify-admin-subject`.

## Verification

From the repository root:

```bash
make helm-chart-sync
make helm-chart-check
```

`make helm-chart-check` runs, in order: realm sync and `auth.adminSubject` checks, schema negative tests (`verify-schema.sh`), `helm lint`, and template render tests (`verify-template.sh`).

Individual targets are available for iteration: `make helm-chart-verify`, `make helm-chart-verify-schema`, `make helm-chart-lint`, `make helm-chart-template`.

Quick checks while iterating:

```bash
helm lint deploy/helm/dcm
python3 -m json.tool deploy/helm/dcm/values.schema.json > /dev/null
```

## Do not

- Auto-generate `values.schema.json` from YAML or templates.
- Strip `values.yaml` comments without moving the information to schema `description` or README.
- Replace `verify-template.sh` or template `fail` checks with schema-only validation.
- Edit `Chart.yaml` version or dependencies unless that is the task.
- Commit secrets or real cluster credentials in values or examples.

## Chart layout

```
deploy/helm/dcm/
├── AGENTS.md              # this file
├── Chart.yaml
├── README.md
├── values.yaml
├── values.schema.json
├── files/realm-export.json   # synced from deploy/keycloak/realm-export.json
├── scripts/
│   ├── verify-schema.sh
│   └── verify-template.sh
└── templates/
```

Makefile targets live in `make/helm.mk` (`helm-chart-sync`, `helm-chart-check`, and the individual `helm-chart-verify*` / `helm-chart-lint` / `helm-chart-template` steps).
