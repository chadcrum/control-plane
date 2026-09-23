# Environment Agent with Kind

Run the [environment-agent](https://github.com/dcm-project/environment-agent) alongside the
control-plane compose stack. The agent shares platform NATS, registers with the control-plane on
the compose network, and runs embedded Service Providers (container, vm, cluster, storage) in-process using
a kubeconfig to reach workloads on Kind.

## Prerequisites

- [Podman](https://podman.io/) or [Docker](https://www.docker.com/) (Makefile auto-detects)
- [Kind](https://kind.sigs.k8s.io/) and `kubectl`
- [utilities](https://github.com/dcm-project/utilities) repo as a sibling directory
  (`../utilities`) for Kind helper scripts
- Kind and compose on the **same** container runtime

## Quick start

This guide uses **two compose steps**:

1. **`make compose-up`** — platform only (creates `control-plane_default` for `kind-connect`).
2. **`make compose-up-with-agent`** — after Kind is wired.

Most Kind prep (`install-kubevirt`, `kubeconfig-for-compose`) can run **before** `compose-up`.
Only **`make kind-connect`** must run after the platform stack is up (compose network must exist).

### 1. Create a Kind cluster

Use the same container runtime as compose (Podman example):

```bash
KIND_EXPERIMENTAL_PROVIDER=podman kind create cluster --name dcm-local
kubectl config use-context kind-dcm-local
```

For in-cluster agent deploy on the same cluster, see the
[environment-agent in-cluster guide](https://github.com/dcm-project/environment-agent/blob/main/deploy/docs/in-cluster.md)
and create the cluster with `../utilities/scripts/kind/kind-local.yaml` (host NodePort mappings for
`k8s-verify`).

### 2. Prepare the cluster

From the control-plane repo root:

```bash
make install-kubevirt          # when vm is in AGENT_EMBEDDED_SPS
make kubeconfig-for-compose
```

Writes `deploy/.kube/config` (API URL `https://kubernetes:6443`). No compose stack required yet.

### 3. Start the platform stack (without the agent)

```bash
cp deploy/.env.example deploy/.env
make compose-up
```

Control-plane API: `http://localhost:8080`. DCM UI: `http://localhost:7007`.

Set agent variables in `deploy/.env` before starting the agent (step 6).

### 4. Connect Kind to compose

Run after `make compose-up` so `control-plane_default` exists:

```bash
make kind-connect
```

Override the utilities path when needed:

```bash
make kind-connect KIND_SCRIPTS_DIR=/path/to/utilities/scripts/kind
```

### 5. Configure the agent

**Required:** set `AGENT_EMBEDDED_SPS` in `deploy/.env` (created in step 3). There is no default value
and an unset value means no embedded service providers.

Add `vm`, `cluster`, `container` or `storage` as needed (e.g. `container,vm`). Run `make install-kubevirt`
when `vm` is included.

### 6. Start the agent (`compose-up-with-agent`)

```bash
make compose-up-with-agent
```

Brings up the environment-agent profile on the running platform stack (or starts platform + agent
together if the stack was stopped).

Agent API: `http://localhost:8081`.

### 7. Verify

```bash
curl http://localhost:8080/api/v1alpha1/health
curl http://localhost:8080/api/v1alpha1/agents
curl http://localhost:8081/api/v1alpha1/health
curl http://localhost:8081/api/v1alpha1/providers
```

### 8. Teardown

```bash
make compose-down              # kind-disconnect, detach externals, compose down, remove networks
kind delete cluster --name dcm-local
```

## Makefile targets

| Target | Purpose |
|--------|---------|
| `compose-up` | Platform stack (postgres, nats, keycloak, control-plane, dcm-ui) |
| `install-kubevirt` | Install KubeVirt on the current Kind cluster |
| `kubeconfig-for-compose` | Write `deploy/.kube/config` for the agent container |
| `kind-connect` | Join Kind to `control-plane_default` |
| `compose-up-with-agent` | Add/start environment-agent on the platform stack |
| `compose-down` | Kind disconnect, detach network members, compose down, remove networks |

Scripts are under `UTILITIES_DIR` (default `../utilities`):

- `scripts/kind/` — Kind networking
- `scripts/compose/` — compose network teardown
- `scripts/kubevirt/` — KubeVirt install

See [utilities scripts/kind/README.md](https://github.com/dcm-project/utilities/blob/main/scripts/kind/README.md)
and [scripts/compose/README.md](https://github.com/dcm-project/utilities/blob/main/scripts/compose/README.md).

## Configuration

Copy `deploy/.env.example` to `deploy/.env` in this repo and set agent profile variables before
`compose-up-with-agent`. See [deploy/.env.example](../.env.example) for the variables wired into
`deploy/compose.yaml`.

For the full catalog of agent and SP environment variables, see
[environment-agent deploy/.env.example](https://github.com/dcm-project/environment-agent/blob/main/deploy/.env.example).

| Variable | Default | Description |
| --- | --- | --- |
| `AGENT_NAME` | `local-agent` | Agent name sent to DCM |
| `AGENT_ENVIRONMENT` | `dev` | Environment classification |
| `AGENT_COST` | `low` | Cost classification |
| `AGENT_PORT` | `8081` | Host port for agent HTTP API |
| `AGENT_EMBEDDED_SPS` | _empty_ (set in `deploy/.env`) | **Required** for the agent profile. Comma-separated: `container`, `vm`, `cluster`, `storage` |
| `AGENT_KUBECONFIG_HOST` | `~/.kube/config` | Host kubeconfig bind mount; use `.kube/config` with Kind (`make kubeconfig-for-compose`) |
| `SP_DEFAULT_KUBECONFIG` | `/kubeconfig` | In-container path (set in `compose.yaml`; do not set in `.env`) |
| `SP_CONTAINER_NAMESPACE` | `default` | Container SP workload namespace |
| `SP_K8S_EXTERNAL_SVC_TYPE` | `NodePort` | Container SP external service type |
| `SP_VM_NAMESPACE` | `default` | VM SP workload namespace |
| `SP_CLUSTER_NAMESPACE` | _(required for cluster SP)_ | ACM cluster namespace |
| `SP_PULL_SECRET` | _(required for cluster SP)_ | Base64-encoded dockerconfigjson |
| `SP_BASE_DOMAIN` | _(none)_ | Base domain for hosted clusters |
| `SP_STORAGE_NAMESPACE` | `default` | Storage SP workload namespace |
| `SP_K8S_DEFAULT_STORAGE_CLASS` | _(none)_ | Default storage class for storage SP |
| `SP_K8S_DEFAULT_ACCESS_MODE` | `ReadWriteOnce` | Default PVC access mode for storage SP |
| `ENVIRONMENT_AGENT_VERSION` | `main` | Agent image tag |

`DCM_REGISTRATION_URL` is set to `http://control-plane:8080` in compose (base URL only and the agent
appends `/api/v1alpha1/agents`). `AGENT_MESSAGING_URL` is `nats://nats:4222`.

## Why Kind networking is needed

| Problem | Cause |
|---------|-------|
| Agent container can't reach Kind's IP | Kind and compose run on separate container networks |
| Agent uses `127.0.0.1:<random-port>` | Default kubeconfig targets the host-side port mapping |
| TLS error with arbitrary hostname | API server cert only includes specific SANs |

`make kind-connect` joins Kind to the compose network with alias `kubernetes` (a cert SAN).
`make kubeconfig-for-compose` rewrites the API server URL to `https://kubernetes:6443`.
