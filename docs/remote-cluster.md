# Remote Cluster Targeting

The dev/test loop is profile-driven. Switching clusters is one variable in
`.env`:

```
CLUSTER_PROFILE=oksg-test      # kind+local (default)
CLUSTER_PROFILE=<your-name>    # k3s on a remote Ubuntu host over SSH
                               # (after copying .env.d/remote-k3s.example.env)
```

`CLUSTER_PROFILE` names a file under `.env.d/<name>.env` that carries the
engine, registry, kubeconfig path, and engine-specific knobs.

## Key Takeaways

For day-to-day use, three things are worth remembering:

1. **One variable switches everything.** `CLUSTER_PROFILE` in `.env` picks
   the engine, registry path, kubeconfig, and (when remote) the SSH host.
   No other file or task needs editing to flip cells.
2. **Image tags never change between cells.** `docker push localhost:5001/…`
   works locally for kind and remotely for k3s+ssh — the SSH tunnel and the
   k3s `registries.yaml` mirror make `localhost:5001` resolve to the same
   logical place on both ends. Remote profiles may set `IMAGE_PLATFORM`
   (for example `linux/amd64`) when the server CPU architecture differs from
   the local Docker daemon.
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
              │   REGISTRY_HOST=...     │
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
| `k3s` + `ssh` (single-node)   | **Tested** by this change       |
| Anything else (k3d, minikube, kubeadm, EKS/GKE, multi-node k3s, etc.) | Out of scope — provider hooks are open, scripts are not |

The two supported cells share the same logical registry URL
(`REGISTRY_HOST=localhost:5001`), so image tags and Helm values are
identical across them.

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
│   │ registry:2              │   "oksg-kind-registry"          │
│   │ 127.0.0.1:5001 → :5000  │◄──┐                             │
│   └─────────────────────────┘   │ image pulls                 │
│            ▲                    │                             │
│            │ kubectl/helm       │                             │
│            │                    │                             │
│   ┌────────┴────────────────────┴──────────────┐              │
│   │ kind cluster "oksg-test"                   │              │
│   │   control-plane node + worker node         │              │
│   │   containerd mirrors localhost:5001 →      │              │
│   │     http://oksg-kind-registry:5000         │              │
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

Nothing to configure — `.env.d/oksg-test.env` is committed and selected by
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
| `registry:2` container      | Laptop (Docker)          | Holds the OKSG/demo images          |
| kind cluster (1 cp + 1 wk)  | Laptop (Docker containers)| Hosts the workloads under test     |
| backend, otel, demo apps    | Laptop (kind cluster)    | The system under test               |
| Vite dev server             | Laptop (Docker)          | Frontend HMR, proxies `/api` to backend port-forward |

## Cell: `k3s` + `ssh` (remote — single-node)

The trick: the image tag stays `localhost:5001/...` on both ends. An SSH
`-L` tunnel on the laptop and `registries.yaml` on the server make
`localhost:5001` resolve to the same registry container that lives on the
server.

```
╭───────────── your laptop ─────────────╮          ╭────────────── remote server ─────────────╮
│                                       │          │                                          │
│  docker push localhost:5001/foo       │          │   ┌──────────────────────────┐           │
│           │                           │          │   │ registry:2 container     │           │
│           ▼                           │          │   │ 127.0.0.1:5001 → :5000   │◄──┐       │
│  ┌────────────────────────┐           │          │   │ "oksg-registry"          │   │       │
│  │ ssh -L 5001:           │           │          │   └──────────────────────────┘   │       │
│  │   localhost:5001       │═══tunnel═══════════════►                                │       │
│  │   <K3S_SSH_HOST>       │           │ tcp:5001 │           ▲                      │       │
│  └────────────────────────┘           │          │           │ image pulls          │       │
│  ▲                                    │          │           │                      │       │
│  │ task test:cluster:tunnel           │          │   ┌───────┴──────────────────────┴───┐   │
│  │ (foreground; keep open in pane 2)  │          │   │ k3s server (containerd)          │   │
│  │                                    │          │   │   registries.yaml mirrors        │   │
│  │  kubectl (via                      │          │   │   localhost:5001 → 127.0.0.1:5001│   │
│  │   ~/.kube/configs/                 │          │   │                                  │   │
│  │   remote-k3s.yaml)                 │          │   │   serves API on :6443            │   │
│  └────────────────────────────────────┼──tcp:6443┼──►│                                  │   │
│                                       │   (direct,│   └──────────────────────────────────┘   │
│  kubeconfig embeds                    │   not via │                                          │
│  https://<K3S_SERVER_IP>:6443         │   tunnel) │                                          │
│                                       │          │                                          │
╰───────────────────────────────────────╯          ╰──────────────────────────────────────────╯
```

Two data paths from the laptop to the server, each doing one thing:

| Path                                         | Purpose                                  |
|----------------------------------------------|------------------------------------------|
| `ssh -L 5001:localhost:5001`                 | `docker push` images so k3s can pull them|
| Direct TCP to `K3S_SERVER_IP:6443`           | `kubectl` / `helm` against the API server|

### Responsibilities (k3s+ssh)

| Component                   | Where it runs       | Responsibility                                |
|-----------------------------|---------------------|-----------------------------------------------|
| Docker daemon (build)       | Laptop              | Builds images locally                         |
| SSH tunnel `-L 5001:…`      | Laptop (foreground) | Pipes `docker push localhost:5001` to server  |
| kubectl / helm              | Laptop              | Talks directly to k3s API on `K3S_SERVER_IP:6443`|
| Vite dev server             | Laptop (Docker)     | Frontend HMR; `/api` → port-forward → API server |
| Docker daemon (runtime)     | Server              | Runs the in-cluster registry container        |
| `registry:2` ("oksg-registry") | Server (Docker)  | Holds the OKSG/demo images                    |
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
   │              --name oksg-registry              ▶ │ registry:2 │
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

Two panes — the tunnel must stay open while you work.

```
       pane 1 (work)                              pane 2 (must stay open)
       ─────────────                              ──────────────────────

 VARIANT=base task test:e2e:deploy                task test:cluster:tunnel
       │ docker build → docker push localhost:5001/…    │ ssh -N -L 5001:…
       │                                                │ (foreground)
       │ kubectl apply                                  │
       │                                                │
 task test:e2e:apply-otel                              │
       │ helm upgrade --install …                       │
       │                                                │
 kubectl port-forward 18081:8080 &                      │
       │                                                │
 task web:dev                                           │
       └─────► Vite (5173) ─/api─► port-forward (18081) ─► API (6443) ─► backend pod
```

Or, copy-pasteable:

```bash
# Pane 2 (must stay open while you work — Ctrl-C to stop):
task test:cluster:tunnel

# Pane 1, one-off after every setup:
task test:e2e:setup           # ensures registry, registries.yaml, kubeconfig
task test:cluster:doctor      # confirms SSH, k3s, registry, tunnel, kubectl, TLS SAN

# Pane 1, normal work:
VARIANT=base task test:e2e:deploy
task test:e2e:apply-otel
task test:e2e:verify
kubectl port-forward -n oksg-system deployment/oksg-backend 18081:8080 &
task web:dev                  # frontend hits the remote cluster
```

### How it works (in words)

- Image tags stay `localhost:5001/oksg-*`. On the dev machine that name is
  forwarded through the SSH tunnel to a `registry:2` container on the
  server. Inside the cluster, k3s containerd mirrors `localhost:5001` to
  `http://127.0.0.1:5001` via `/etc/rancher/k3s/registries.yaml`, which
  resolves to the same container.
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
- `oksg-registry container not running` — re-run `task test:e2e:setup`.
- `localhost:5001 not reachable from this host` — start the tunnel
  (`task test:cluster:tunnel`) in another pane.
- `kubectl unreachable` — usually a TLS SAN mismatch. The doctor prints
  the exact `--tls-san=...` remediation to apply on the server.

## Tearing down

```bash
task test:e2e:teardown        # removes Helm release + demo namespaces (k3s),
                              # or destroys the kind cluster (kind)
./test/e2e/cluster/k3s/teardown.sh --full
                              # (k3s only) also removes the oksg-registry container.
                              # k3s itself is never touched by these scripts.
```
