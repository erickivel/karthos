#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
cd "${REPO_ROOT}"

if [[ -z "${KARTHOS_IMAGE_TAG_OVERRIDE:-}" ]]; then
  KARTHOS_IMAGE_TAG_OVERRIDE="$("${SCRIPT_DIR}/image-tag.sh")"
fi
export KARTHOS_IMAGE_TAG_OVERRIDE
# shellcheck disable=SC1091
. "${REPO_ROOT}/test/e2e/cluster/profile.sh"

export KIND_NAMESPACE="${KIND_NAMESPACE:-karthos-system}"
export HELM_RELEASE_NAME="${HELM_RELEASE_NAME:-karthos}"
export KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-karthos-test}"
export E2E_OUTPUT_DIR="${E2E_OUTPUT_DIR:-${REPO_ROOT}/bin/e2e}"
export PROFILE_PATH="${PROFILE_PATH:-${E2E_OUTPUT_DIR}/profile.json}"
export VALUES_PATH="${VALUES_PATH:-${E2E_OUTPUT_DIR}/otel-values.yaml}"
export RENDERED_PATH="${RENDERED_PATH:-${E2E_OUTPUT_DIR}/rendered}"

"${SCRIPT_DIR}/ensure-cluster.sh"
mkdir -p "${E2E_OUTPUT_DIR}"

image_builder="${KARTHOS_DEPLOY_IMAGE_BUILDER:-${REPO_ROOT}/test/e2e/prepare-platform-images.sh}"
"${image_builder}"

go run ./cmd/discovery-job --mode=discover \
  --kubeconfig="${KUBECONFIG}" \
  --cluster-name="${CLUSTER_PROFILE}" \
  --output="${PROFILE_PATH}" \
  --namespace="${KIND_NAMESPACE}" \
  --target-namespace=karthos-demo

helm dependency build ./deploy/helm/karthos
go run ./cmd/discovery-job --mode=deploy \
  --capability-profile="${PROFILE_PATH}" \
  --values-output="${VALUES_PATH}" \
  --rendered-output="${RENDERED_PATH}" \
  --namespace="${KIND_NAMESPACE}" \
  --chart-path=./deploy/helm/karthos \
  --release-name="${HELM_RELEASE_NAME}" \
  --backend-image-repository="${REGISTRY_HOST}/karthos-backend" \
  --backend-image-tag="${IMAGE_TAG}" \
  --backend-image-pull-policy=Always \
  --frontend-image-repository="${REGISTRY_HOST}/karthos-frontend" \
  --frontend-image-tag="${IMAGE_TAG}" \
  --frontend-image-pull-policy=Always \
  --storage-backend=external \
  --postgresql-mode=bundled \
  --victoriametrics-mode=bundled

"${SCRIPT_DIR}/verify.sh"
printf 'deployed revision=%s tag=%s\n' "$(git rev-parse --short=7 HEAD)" "${IMAGE_TAG}"
