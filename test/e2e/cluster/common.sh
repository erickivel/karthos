#!/usr/bin/env bash
# Source me from any test/e2e/cluster/**/*.sh.
# Resolves repo root, cd's there, and sources the active cluster profile.
#
# After sourcing, the caller can rely on:
#   - cwd is the repo root
#   - profile.sh has been sourced (CLUSTER_PROFILE / CLUSTER_ENGINE / ... exported)

__OKSG_COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${__OKSG_COMMON_DIR}/../../.." && pwd)"
# shellcheck disable=SC2164
cd "${REPO_ROOT}"
# shellcheck disable=SC1091
. "${__OKSG_COMMON_DIR}/profile.sh"
