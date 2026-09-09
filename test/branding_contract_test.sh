#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
legacy_prefix="$(printf '%s%s' 'ok' 'sg')"
legacy_repository="$(printf '%s-%s-%s-%s' 'observability' 'driven' 'kubernetes' 'system-graph')"
legacy_pattern="${legacy_prefix}|${legacy_repository}"

legacy_paths="$(git -C "${repo_root}" ls-files | rg -i "${legacy_pattern}" || true)"
legacy_content="$(git -C "${repo_root}" grep -n -i -E "${legacy_pattern}" || true)"

if [[ -n "${legacy_paths}" || -n "${legacy_content}" ]]; then
  echo "tracked legacy brand references remain" >&2
  [[ -z "${legacy_paths}" ]] || printf '%s\n' "${legacy_paths}" >&2
  [[ -z "${legacy_content}" ]] || printf '%s\n' "${legacy_content}" >&2
  exit 1
fi

test -f "${repo_root}/web/public/karthos-mark.svg"
cmp -s "${repo_root}/web/public/karthos-mark.svg" "${repo_root}/web/public/favicon.svg"
rg -Fq '<title>Karthos - Kubernetes Data Foundation</title>' "${repo_root}/web/index.html"
rg -Fq 'karthos-mark.svg' "${repo_root}/web/src/components/Shell/TopBar.tsx"
