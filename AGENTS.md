# Agent Guide

This repository builds an observability-driven Kubernetes system graph. See [README.md](README.md) for a project summary and [GUIDELINE.md](GUIDELINE.md) for design principles that govern all contributions.

## Quick Reference Commands

- `task ci` -- run before and after every meaningful change
- `task test` -- unit tests only
- `task fmt` -- format code
- `task build` -- compile all binaries
- `task generate` -- regenerate JSON Schema output after schema changes

## Documentation Map

All implementation details live in `docs/`. Use and update these files when implementation changes.

| Document | What it covers | Update when... |
|----------|---------------|----------------|
| [README.md](README.md) | Project summary, phase status, getting started | Project phase, runtime surface, or entrypoints change |
| [GUIDELINE.md](GUIDELINE.md) | Design principles, configuration rules, dependency philosophy | Principles themselves change (rare) |
| [docs/architecture.md](docs/architecture.md) | System layers, component boundaries, deployment model | Adding or modifying adapters or layers |
| [docs/discovery-flow.md](docs/discovery-flow.md) | Discovery pipeline, sequence diagrams, example outputs | Changing discovery or deployment pipeline |
| [docs/runtime-flow.md](docs/runtime-flow.md) | Post-deploy telemetry flow, component responsibilities | Changing the deployed runtime telemetry path |
| [docs/testing.md](docs/testing.md) | Unit tests, e2e model, scenarios, traffic generation | Changing test infrastructure, task names, or Helm install flow |
| [docs/api-reference.md](docs/api-reference.md) | REST API routes, query params, request/response examples | Adding, removing, or changing API endpoints |
| [docs/development.md](docs/development.md) | Project structure, tech stack, common commands, Helm ownership | Adding packages, changing structure, or modifying workflows |

## After-Change Checklist

1. Run `task ci`.
2. Ensure unit tests cover any new behavior.
3. If you changed discovery or collection behavior, run the relevant e2e scenarios (see [docs/testing.md](docs/testing.md) for commands and expectations).
4. Update the documentation file(s) from the table above that correspond to what you changed.
5. Remove or rewrite any docs, commands, or diagrams made stale by your change -- in the same commit.

## CI/CD

GitHub Actions runs on PRs and pushes to `main`/`development`:

- **lint**, **format-check**, **vet**, **test**, **build** on all branches
- **security** (govulncheck + gosec) on `main` only

Run `task ci` locally before pushing.

## Upgrading the Go Version

When upgrading Go (e.g., to fix `govulncheck` findings), update **all** of these files:

| File | What to change |
|------|----------------|
| `go.mod` | `go X.Y.Z` directive |
| `.tool-versions` | `golang X.Y.Z` (asdf) |
| `.github/workflows/ci.yml` | `GO_VERSION: "X.Y.Z"` env var |
| `.env.example` | `GO_VERSION` comment |
| `deploy/images/backend/Dockerfile` | `FROM golang:X.Y.Z-alpine` |
| `test/e2e/apps/base/gateway-go/Dockerfile` | `FROM golang:X.Y.Z-alpine` |
| `test/e2e/apps/with-prometheus/gateway-go/Dockerfile` | `FROM golang:X.Y.Z-alpine` |
| `test/e2e/apps/with-prometheus/gateway-go/go.mod` | `go X.Y.Z` directive |

After updating, run `go mod tidy`, reinstall tools (`go install ...`), `asdf reshim golang`, then `task ci` and `task security`.

## Frontend Development Rules

The frontend lives in `web/`. **All frontend operations run inside Docker containers — never install or run npm/node locally.**

### Commands

| Command | What it does |
|---------|-------------|
| `task web:dev` | Start dev container with Vite HMR (port 5173) |
| `task web:build` | Build production Docker image |
| `task web:install` | Install npm dependencies inside container |
| `task web:typecheck` | Run `tsc --noEmit` inside container |
| `task web:lint` | Run ESLint inside container |

### Key rules

- **Never run `npm install`, `npm run`, `npx`, or any node/npm command on the host machine.** Always use Docker (`docker compose run --rm oksg-frontend <command>` or the Taskfile shortcuts).
- Source files are volume-mounted into the dev container for HMR. Edit files on the host, the container picks up changes.
- API requests are proxied: Vite dev server proxies `/api/*` to the backend container, nginx does the same in production.
- The `web/` directory has its own `Taskfile.yml` included from the root.

## Kubernetes Port-Forwarding

When you need to access cluster services locally, use these commands:

| Service | Command |
|---------|---------|
| OKSG Backend API | `kubectl port-forward -n oksg-system deployment/oksg-backend 18081:8080` |

The backend API is then available at `http://localhost:18081/api/v1/...`.

## Documentation Maintenance Rules

- Treat `docs/` as the primary implementation documentation surface.
- Keep documentation aligned with actual implementation, not aspirational or outdated flows.
- Keep diagrams (ASCII and Mermaid) synchronized with implementation.
- Keep `docs/discovery-flow.md` explicit about what auto-detection does today versus what the planner accepts from externally supplied profiles.
- Keep logging documentation in `docs/`: structured JSON logs to stdout only, `kubectl logs` as the debugging path.
- Update example outputs in `docs/discovery-flow.md` when ClusterProfile structure changes.

## Telemetry Mapping Rules

- Before adding or remapping any metric, trace-derived metric, or telemetry source, verify the upstream or source-of-truth docs/spec first.
- Record the signal contract you verified: how the signal is generated, whether it is gauge/sum/histogram and cumulative or delta, its units, and any default rate, flush, or aggregation window semantics.
- Only after that verification should you change the semconv metric registry, dialect translations, unit conversions, query-time derivations, storage expectations, or UI assumptions.
- Do not infer telemetry semantics from a metric name alone. Connectors such as `spanmetrics` and `servicegraph` can emit counters, histograms, and unit defaults that differ from what the semconv name suggests.
