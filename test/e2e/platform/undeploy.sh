#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
cd "${REPO_ROOT}"
# shellcheck disable=SC1091
. "${REPO_ROOT}/test/e2e/cluster/profile.sh"

namespace="${KIND_NAMESPACE:-karthos-system}"
release="${HELM_RELEASE_NAME:-karthos}"
helm uninstall "${release}" --namespace "${namespace}" || true
kubectl delete pvc "data-${release}-postgresql-0" --namespace "${namespace}" --ignore-not-found
