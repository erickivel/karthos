#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"
# shellcheck disable=SC1091
. "${REPO_ROOT}/test/e2e/cluster/profile.sh"

build_and_load() {
  local tag="$1"
  local context="$2"
  local dockerfile="${3:-}"
  local platform_args=()
  if [[ -n "${IMAGE_PLATFORM:-}" ]]; then
    platform_args=(--platform "${IMAGE_PLATFORM}")
  fi
  if [[ -n "${dockerfile}" ]]; then
    docker build "${platform_args[@]+"${platform_args[@]}"}" -t "${tag}" -f "${dockerfile}" "${context}"
  else
    docker build "${platform_args[@]+"${platform_args[@]}"}" -t "${tag}" "${context}"
  fi
  "${REPO_ROOT}/test/e2e/cluster/image.sh" push_or_load "${tag}"
}

build_and_load "${REGISTRY_HOST}/karthos-backend:${IMAGE_TAG}" "${REPO_ROOT}" "${REPO_ROOT}/deploy/images/backend/Dockerfile"
build_and_load "${REGISTRY_HOST}/karthos-frontend:${IMAGE_TAG}" "${REPO_ROOT}/web"
