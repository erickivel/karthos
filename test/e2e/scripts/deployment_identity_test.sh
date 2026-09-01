#!/usr/bin/env bash

set -euo pipefail

TASKFILE="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)/Taskfile.test.yml"
for runner in \
  prometheus-available/run.sh \
  istio-available/run.sh \
  source-contract-rejection/run.sh; do
  expected="HELM_RELEASE_NAME={{.HELM_RELEASE_NAME}} KIND_NAMESPACE={{.KIND_NAMESPACE}} ./test/e2e/scenarios/${runner}"
  if ! grep -Fq "${expected}" "${TASKFILE}"; then
    echo "expected ${runner} to receive release and namespace identity" >&2
    exit 1
  fi
done

echo "E2E deployment identity dispatch: PASS"
