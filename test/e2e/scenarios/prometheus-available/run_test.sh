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

echo "Prometheus raw counter selector units: PASS"
