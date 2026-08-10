#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t oksg-detector-failure.XXXXXX)"
trap 'rm -rf "${TMPDIR_TEST}"' EXIT

"${REPO_ROOT}/bin/discovery-job" --mode=discover \
  --kubeconfig="${TMPDIR_TEST}/unusable-kubeconfig" \
  --cluster-name=detector-failure \
  --output="${TMPDIR_TEST}/profile.json" \
  --namespace=oksg-system \
  --target-namespace=oksg-demo >/dev/null 2>&1 && {
  echo "detector failure unexpectedly produced a capability profile" >&2
  exit 1
}

test ! -e "${TMPDIR_TEST}/profile.json"
echo "Detector failure is fail-closed: PASS"
