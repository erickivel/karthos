#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
RUNNER="${REPO_ROOT}/test/e2e/scenarios/prometheus-available/run.sh"

# VictoriaMetrics stores the canonical servicegraph counter in its physical base
# unit. Rates are derived and asserted through the public API later in the runner.
if grep -Fq 'oksg_unit="per_second"' "${RUNNER}"; then
  echo "raw VictoriaMetrics selectors must not require the API rate unit" >&2
  exit 1
fi

raw_count_selectors="$(grep -Fc 'oksg_unit="count"' "${RUNNER}")"
if [[ "${raw_count_selectors}" -ne 2 ]]; then
  echo "expected both raw VictoriaMetrics selectors to require oksg_unit=count, got ${raw_count_selectors}" >&2
  exit 1
fi

if ! grep -Fq 'def supported_api_results_ready(' "${RUNNER}"; then
  echo "expected runner to define a complete supported API result readiness check" >&2
  exit 1
fi

for result in request_increases error_increases request_rates error_rates durations; do
  if ! grep -Fq "${result}" "${RUNNER}"; then
    echo "expected readiness check to include ${result}" >&2
    exit 1
  fi
done

if ! grep -Fq 'if supported_api_results_ready(' "${RUNNER}"; then
  echo "expected runner to poll until all supported API results are ready" >&2
  exit 1
fi

for route in \
  'NAMESPACE="${KIND_NAMESPACE:-oksg-system}"' \
  'BACKEND_DEPLOYMENT="${HELM_RELEASE_NAME:-oksg-otel}-backend"' \
  'VM_SERVICE="${HELM_RELEASE_NAME:-oksg-otel}-victoria-metrics-single-server"' \
  'kubectl port-forward -n "${NAMESPACE}" deployment/${BACKEND_DEPLOYMENT}' \
  'kubectl port-forward -n "${NAMESPACE}" svc/${VM_SERVICE}'; do
  if ! grep -Fq "${route}" "${RUNNER}"; then
    echo "expected runner to derive collector-plane route: ${route}" >&2
    exit 1
  fi
done

echo "Prometheus scenario contract checks: PASS"
