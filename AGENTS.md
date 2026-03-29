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

## Documentation Maintenance Rules

- Treat `docs/` as the primary implementation documentation surface.
- Keep documentation aligned with actual implementation, not aspirational or outdated flows.
- Keep diagrams (ASCII and Mermaid) synchronized with implementation.
- Keep `docs/discovery-flow.md` explicit about what auto-detection does today versus what the planner accepts from externally supplied profiles.
- Keep logging documentation in `docs/`: structured JSON logs to stdout only, `kubectl logs` as the debugging path.
- Update example outputs in `docs/discovery-flow.md` when ClusterProfile structure changes.
