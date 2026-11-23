#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "${REPO_ROOT}"

git_sha="${KARTHOS_GIT_SHA_OVERRIDE:-$(git rev-parse --short=7 HEAD)}"
short_sha="${git_sha:0:7}"

if [[ -n "${KARTHOS_GIT_DIRTY_OVERRIDE:-}" ]]; then
  dirty="${KARTHOS_GIT_DIRTY_OVERRIDE}"
elif [[ -n "$(git status --porcelain --untracked-files=normal)" ]]; then
  dirty=true
else
  dirty=false
fi

if [[ "${dirty}" == "true" ]]; then
  timestamp="${KARTHOS_DIRTY_TIMESTAMP_OVERRIDE:-$(date -u +%Y%m%dT%H%M%SZ)}"
  printf '%s-dirty-%s\n' "${short_sha}" "${timestamp}"
else
  printf '%s\n' "${short_sha}"
fi
