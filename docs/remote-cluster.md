# Remote Cluster Targeting

The dev/test loop is profile-driven. Switching clusters is one variable in
`.env`:

```
CLUSTER_PROFILE=karthos-test      # kind+local (default)
CLUSTER_PROFILE=<your-name>    # k3s on a remote Ubuntu host over SSH
                               # (after copying .env.d/remote-k3s.example.env)
```

`CLUSTER_PROFILE` names a file under `.env.d/<name>.env` that carries the
engine, kubeconfig path, and engine-specific knobs. The E2E registry is always
`localhost:5001` in both supported cells.

## Key Takeaways

For day-to-day use, three things are worth remembering:

1. **One variable switches everything.** `CLUSTER_PROFILE` in `.env` picks
   the engine, kubeconfig, and (when remote) the SSH host.
   No other file or task needs editing to flip cells.
2. **Image tags never change between cells.** kind pushes to its local
   registry. For k3s+ssh, the harness streams the locally built image over
   SSH, loads and pushes it from the server, then verifies the server
   registry manifest digest before deployment. The SSH tunnel is optional
   for manual registry inspection, not required to deploy images. Remote
   profiles may set `IMAGE_PLATFORM` (for example `linux/amd64`) when the
   server CPU architecture differs from the local Docker daemon.
3. **The frontend dev loop is identical** for both cells. `task web:dev`
   talks to the backend through `kubectl port-forward`, which proxies via
   the Kubernetes API server. The API server's location (laptop vs remote)
   is transparent to it.

## How the profile is resolved

```
┌────────────────────┐
│ .env               │
│  CLUSTER_PROFILE=X │───┐
└────────────────────┘   │ picks
                         ▼
              ┌─────────────────────────┐
              │ .env.d/X.env            │
              │   CLUSTER_ENGINE=...    │   (kind | k3s)
              │   CLUSTER_LOCATION=...  │   (local | ssh)
              │   KUBECONFIG=...        │
              │   IMAGE_TAG=...         │
              │   [engine-specific]     │
              └────────────┬────────────┘
                           │ sourced by
                           ▼
              ┌──────────────────────────────┐
              │ test/e2e/cluster/profile.sh  │  resolver — exports the vars
              └────────────┬─────────────────┘
                           │ used by
                           ▼
              ┌──────────────────────────────┐
              │ Taskfile + provider scripts  │
              │  (dispatch on CLUSTER_ENGINE)│
              └──────────────────────────────┘
```

Every cluster-touching task dispatches on `CLUSTER_ENGINE`:

```
                      ┌─────────────────┐
 task test:e2e:setup  │ CLUSTER_ENGINE? │
                      └────────┬────────┘
                  kind ◄───────┴───────► k3s
                   │                      │
                   ▼                      ▼
   test/e2e/kind/setup.sh         test/e2e/cluster/k3s/setup.sh
   (creates kind cluster +        (provisions registry on server,
    registry container)            installs registries.yaml,
                                   exports kubeconfig)
```

Same pattern for `e2e:teardown`, `e2e:ensure-cluster`, `cluster:doctor`,
`cluster:tunnel`. Adding a new engine = new `test/e2e/cluster/<engine>/`
directory + new case in those branches; nothing else moves.

## Support Matrix

| Cell                          | Status                          |
|-------------------------------|---------------------------------|
| `kind` + `local`              | **Tested** (default; current behaviour) |
| `k3s` + `ssh` (single-node)   | **Supported and tested**        |
| Anything else (k3d, minikube, kubeadm, EKS/GKE, multi-node k3s, etc.) | Out of scope — provider hooks are open, scripts are not |

The two supported cells use the fixed registry URL `localhost:5001`; profiles
cannot override it. Image tags and Helm values are therefore identical across
the supported cells.

## Cell: `kind` + `local` (default)

Everything runs on the laptop. A `registry:2` container holds images; kind's
containerd mirrors `localhost:5001` to it.

```
╭───────────────────────── your laptop ─────────────────────────╮
│                                                               │
│   docker build → docker push                                  │
│            │                                                  │
│            ▼                                                  │
│   ┌─────────────────────────┐                                 │
│   │ registry:2              │   "karthos-kind-registry"          │
│   │ 127.0.0.1:5001 → :5000  │◄──┐                             │
│   └─────────────────────────┘   │ image pulls                 │
│            ▲                    │                             │
│            │ kubectl/helm       │                             │
│            │                    │                             │
│   ┌────────┴────────────────────┴──────────────┐              │
│   │ kind cluster "karthos-test"                   │              │
│   │   control-plane node + worker node         │              │
│   │   containerd mirrors localhost:5001 →      │              │
│   │     http://karthos-kind-registry:5000         │              │
│   └────────────────────────────────────────────┘              │
│            ▲                                                  │
│            │ kubectl port-forward 18081:8080                  │
│            │                                                  │
│   ┌────────┴────────┐                                         │
│   │ task web:dev    │  (Vite dev server, port 5173)           │
│   └─────────────────┘                                         │
│                                                               │
╰───────────────────────────────────────────────────────────────╯
```

Nothing to configure — `.env.d/karthos-test.env` is committed and selected by
default. The standard flow is unchanged:

```bash
task test:e2e:setup
task test:e2e:scenario:fallback
task web:dev
```

### Responsibilities (kind+local)

| Component                   | Where it runs            | Responsibility                      |
|-----------------------------|--------------------------|-------------------------------------|
| Docker daemon               | Laptop                   | Builds images, runs the registry container, runs kind nodes |
| `registry:2` container      | Laptop (Docker)          | Holds the Karthos/demo images          |
| kind cluster (1 cp + 1 wk)  | Laptop (Docker containers)| Hosts the workloads under test     |
| backend, otel, demo apps    | Laptop (kind cluster)    | The system under test               |
| Vite dev server             | Laptop (Docker)          | Frontend HMR, proxies `/api` to backend port-forward |

## Cell: `k3s` + `ssh` (remote — single-node)

The image tag stays `localhost:5001/...` on both ends, but image transport is
executed from the server side. The harness streams `docker save` over SSH to
`docker load`, runs `docker push` on the server, and compares that push digest
with the manifest digest served from the server loopback registry. k3s then
pulls through its `registries.yaml` mirror.

```
╭───────────── your laptop ─────────────╮          ╭────────────── remote server ─────────────╮
│                                       │          │                                          │
│  docker save localhost:5001/foo       │          │   docker load → docker push          │   │
│           │                           │          │           │                          │   │
│           └──── SSH stdin stream ─────────────────►           ▼                          │   │
│                                       │          │   ┌──────────────────────────┐           │
│  optional ssh -L (inspection only)   │          │   │ registry:2 container     │◄──┐       │
│                                       │          │   │ 127.0.0.1:5001 → :5000   │   │       │
│  kubectl (via                         │          │   │ "karthos-registry"          │   │       │
│  ~/.kube/configs/                     │          │   └──────────────────────────┘   │       │
│  remote-k3s.yaml)                     │          │           ▲ image pulls             │       │
│                                       │          │   ┌───────┴──────────────────────┴───┐   │
│                                       │          │   │ k3s server (containerd)          │   │
│                                       │          │   │   registries.yaml mirrors        │   │
│                                       │          │   │   localhost:5001 → 127.0.0.1:5001│   │
│                                       │          │   │   serves API on :6443            │   │
│  └────────────────────────────────────┼──tcp:6443┼──►│                                  │   │
│                                       │   (direct,│   └──────────────────────────────────┘   │
│  kubeconfig embeds                    │   not via │                                          │
│  https://<K3S_SERVER_IP>:6443         │   tunnel) │                                          │
│                                       │          │                                          │
╰───────────────────────────────────────╯          ╰──────────────────────────────────────────╯
```

The required image and API paths, plus the optional inspection path:

| Path                                         | Purpose                                  |
|----------------------------------------------|------------------------------------------|
| `docker save \| ssh <host> docker load`       | Streams the local image to the server without an archive |
| Server-side `docker push localhost:5001/...` | Publishes to the registry k3s can pull from and yields the digest to verify |
| Direct TCP to `K3S_SERVER_IP:6443`           | `kubectl` / `helm` against the API server|
| Optional `ssh -L 5001:localhost:5001`        | Lets a developer inspect the remote registry from the laptop |

### Responsibilities (k3s+ssh)

| Component                   | Where it runs       | Responsibility                                |
|-----------------------------|---------------------|-----------------------------------------------|
| Docker daemon (build)       | Laptop              | Builds images locally and streams `docker save` over SSH |
| SSH tunnel `-L 5001:…`      | Laptop (optional)   | Exposes the remote registry locally for inspection |
| kubectl / helm              | Laptop              | Talks directly to k3s API on `K3S_SERVER_IP:6443`|
| Vite dev server             | Laptop (Docker)     | Frontend HMR; `/api` → port-forward → API server |
| Docker daemon (runtime)     | Server              | Loads streamed images, pushes them, and runs the in-cluster registry container |
| `registry:2` ("karthos-registry") | Server (Docker)  | Holds the Karthos/demo images                    |
| k3s (single-node)           | Server              | Hosts the workloads under test                |
| backend, otel, demo apps    | Server (k3s)        | The system under test                         |

If the laptop and remote node architectures differ, set `IMAGE_PLATFORM` in
the remote profile. For example, Apple Silicon Docker builds `linux/arm64` by
default, while a typical Ubuntu k3s server is `linux/amd64`; without the
override, pods can fail with `exec format error`.

### One-time prerequisites on the server

1. k3s is already installed and active (`systemctl is-active k3s`).
2. k3s is started with a `--tls-san` for the IP you'll embed in the
   exported kubeconfig. If you discover later that `kubectl` errors with
   a cert mismatch, run `task test:cluster:doctor` — it detects this and
   prints the exact remediation:

   ```bash
   # On the server, edit the k3s systemd unit drop-in (or the original
   # ExecStart) to add: --tls-san=<K3S_SERVER_IP>
   sudo systemctl daemon-reload && sudo systemctl restart k3s
   ```

3. SSH key-based auth is set up; `ssh <host> true` succeeds without
   prompting.
4. Docker is installed on the server (used to run the in-cluster registry
   container).

### Activate the profile

Pick a short name for your server (e.g. the SSH alias) and use it as
`<your-profile>`:

```bash
cp .env.d/remote-k3s.example.env .env.d/<your-profile>.env
$EDITOR .env.d/<your-profile>.env        # fill in K3S_SSH_HOST + K3S_SERVER_IP
echo CLUSTER_PROFILE=<your-profile> > .env
```

### What `task test:e2e:setup` does (k3s cell)

The one-time bootstrap. After it runs, `~/.kube/configs/<profile>.yaml`
exists on the laptop and an idempotent registry runs on the server.

```
 Laptop                                                Server
 ──────                                                ──────

 task test:e2e:setup
   │
   │ ssh <host> true                                ▶ (reachability)
   │ ssh <host> "systemctl is-active k3s"           ▶ (k3s alive)
   │
   │ ssh <host> "docker run -d ...                  ▶ ┌────────────┐
   │              --name karthos-registry              ▶ │ registry:2 │
   │              -p 127.0.0.1:5001:5000            ▶ │ on :5001   │
   │              registry:2"                         └────────────┘
   │
   │ scp registries.yaml      ───────────────────►  /tmp/...yaml
   │ scp server-setup.sh      ───────────────────►  /tmp/...sh
   │
   │ ssh -t <host> "bash /tmp/server-setup.sh"      ◄── ONE sudo prompt
   │                                                 ▶ install /etc/rancher/k3s/
   │                                                 ▶   registries.yaml
   │                                                 ▶ systemctl restart k3s
   │                                                 ▶ sudo install -o $USER
   │                                                 ▶   /etc/rancher/k3s/k3s.yaml
   │                                                 ▶   /tmp/kubeconfig-...yaml
   │
   │ scp <host>:/tmp/kubeconfig-...yaml ──────────◄
   │   │ sed "s#127.0.0.1:6443#K3S_SERVER_IP:6443#"
   │   ▼
   │ ~/.kube/configs/<profile>.yaml  (chmod 600)
   ▼
 ready
```

### Daily flow (after setup)

The normal deploy flow needs no registry tunnel. Open one only when you want
to inspect the remote registry from the laptop.

```
       pane 1 (work)                              optional pane 2
       ─────────────                              ───────────────

 VARIANT=base task test:e2e:deploy                task test:cluster:tunnel
       │ fixture docker build → docker save → SSH load/push │ ssh -N -L 5001:…
       │                                                │ (registry inspection only)
       │ kubectl apply                                  │
       │                                                │
 task test:e2e:apply-otel                              │
       │ platform image build/publish → discovery-job --mode=deploy → Helm │
       │                                                │
 kubectl port-forward 18081:8080 &                      │
       │                                                │
 task web:dev                                           │
       └─────► Vite (5173) ─/api─► port-forward (18081) ─► API (6443) ─► backend pod
```

Or, copy-pasteable:

```bash
# One-off after every setup:
task test:e2e:setup           # ensures registry, registries.yaml, kubeconfig
task test:cluster:doctor      # confirms SSH, k3s, server registry, kubectl, TLS SAN

# Pane 1, normal work:
VARIANT=base task test:e2e:deploy
# apply-otel publishes the backend/frontend images through this cell first.
task test:e2e:apply-otel
task test:e2e:verify
kubectl port-forward -n karthos-system deployment/karthos-backend 18081:8080 &
task web:dev                  # frontend hits the remote cluster

# Optional: expose the remote registry locally for manual inspection.
task test:cluster:tunnel
```

### How it works (in words)

- Image tags stay `localhost:5001/karthos-*`. For k3s, `image.sh` streams each
  local image to the remote Docker daemon, pushes it from that daemon, and
  verifies the pushed digest against the remote registry's
  `Docker-Content-Digest` response. No laptop registry tunnel is involved.
  Inside the cluster, k3s containerd mirrors `localhost:5001` to the server
  loopback registry via `/etc/rancher/k3s/registries.yaml`.
- The exported kubeconfig embeds `https://${K3S_SERVER_IP}:${K3S_API_PORT}`
  as the server URL, so `kubectl` reaches the API server directly (no
  tunnel needed for the API).
- `kubectl port-forward` proxies through the API server, so it works for
  the backend and frontend exactly as it does for the kind cell.

### Why single-node

`registries.yaml` on the server points containerd at a registry container
that listens on `127.0.0.1`. That works on a single node — the node and
the registry are the same host. A multi-node k3s cluster would need every
node to either run an equivalent registry mirror or reach a shared one,
which is out of scope here.

## Troubleshooting

```bash
task test:cluster:profile     # print the resolved profile variables
task test:cluster:doctor      # engine-specific pre-flight checks
```

Common failures and what `doctor` says:

- `ssh <host> not reachable` — check `~/.ssh/config` and your key.
- `k3s service not active` — `ssh <host> sudo systemctl status k3s`.
- `registries.yaml missing` — re-run `task test:e2e:setup`.
- `karthos-registry container not running` — re-run `task test:e2e:setup`.
- `remote registry not reachable from the k3s server` — re-run
  `task test:e2e:setup`; a laptop tunnel does not repair this condition.
- `kubectl unreachable` — usually a TLS SAN mismatch. The doctor prints
  the exact `--tls-san=...` remediation to apply on the server.

## Tearing down

```bash
task test:e2e:teardown        # removes Helm release + demo namespaces (k3s),
                              # or destroys the kind cluster (kind)
./test/e2e/cluster/k3s/teardown.sh --full
                              # (k3s only) also removes the karthos-registry container.
                              # k3s itself is never touched by these scripts.
```
