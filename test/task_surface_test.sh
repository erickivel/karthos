#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

expected_tasks="$(cat <<'TASKS'
build
ci
clean
deploy
fmt
fmt:check
generate
lint
security
test
test:branding
test:cluster:doctor
test:cluster:profile
test:cluster:tunnel
test:e2e:debug:api
test:e2e:debug:logs
test:e2e:reset
test:e2e:scenario:detector-failure
test:e2e:scenario:fallback
test:e2e:scenario:istio
test:e2e:scenario:prometheus
test:e2e:scenario:source-contract-rejection
test:e2e:setup
test:e2e:teardown
test:e2e:traffic
test:e2e:traffic:watch
test:fixtures
test:postgres
test:race
tidy
undeploy
vet
web:check
web:dev
TASKS
)"

actual_tasks="$(task --list-all --json | jq -r '.tasks[].name' | sort)"
if [[ "${actual_tasks}" != "${expected_tasks}" ]]; then
  echo "public task surface mismatch" >&2
  diff -u <(printf '%s\n' "${expected_tasks}") <(printf '%s\n' "${actual_tasks}") >&2 || true
  exit 1
fi

if task --summary default >/dev/null 2>&1; then
  echo "default task must not exist" >&2
  exit 1
fi

for task_name in fmt vet tidy lint; do
  dry_run="$(task --dry "${task_name}")"
  [[ "${dry_run}" == *instrumented-gateway* || "${dry_run}" == *fixture:instrumented-gateway* ]]
  [[ "${dry_run}" == *mesh-protocol* ]]
done

test_dry_run="$(task --dry test)"
[[ "${test_dry_run}" == *'go test '* ]]
[[ "${test_dry_run}" != *branding* ]]

build_dry_run="$(task --dry build)"
[[ "${build_dry_run}" == *'go build -o ./bin/backend'* ]]
[[ "${build_dry_run}" == *'go build -o ./bin/discovery-job'* ]]
[[ "${build_dry_run}" != *gofmt* ]]

for scenario in prometheus source-contract-rejection; do
  scenario_dry_run="$(task --dry "test:e2e:scenario:${scenario}" 2>&1)"
  cleanup_count="$(grep -Fc './test/e2e/platform/undeploy.sh' <<<"${scenario_dry_run}" || true)"
  if [[ "${cleanup_count}" -ne 1 ]]; then
    echo "scenario ${scenario} must run exactly one final environment cleanup" >&2
    exit 1
  fi
  if [[ "${scenario_dry_run}" != *'./test/e2e/cluster/istio.sh reset-if-owned'* ]]; then
    echo "scenario ${scenario} cleanup must preserve externally managed Istio" >&2
    exit 1
  fi
done
istio_dry_run="$(task --dry test:e2e:scenario:istio 2>&1)"
istio_steps=(
  './test/e2e/cluster/istio.sh reset-if-owned'
  './test/e2e/platform/undeploy.sh'
  'kubectl delete namespace karthos-demo'
  'kubectl delete namespace karthos-sparse'
  './test/e2e/cluster/istio.sh install'
  './test/e2e/apps/deploy.sh with-istio'
  './test/e2e/scenarios/istio-available/run.sh'
)
previous_line=0
for step in "${istio_steps[@]}"; do
  count="$(grep -Fc "${step}" <<<"${istio_dry_run}" || true)"
  line="$(grep -nF "${step}" <<<"${istio_dry_run}" | head -1 | cut -d: -f1 || true)"
  if [[ "${count}" -ne 1 || "${line:-0}" -le "${previous_line}" ]]; then
    echo "Istio scenario must reset owned resources once before preparation and retain the new environment: ${step}" >&2
    exit 1
  fi
  previous_line="${line}"
done
if grep -Eq './test/e2e/cluster/istio.sh reset$' <<<"${istio_dry_run}"; then
  echo "Istio scenario must preserve an externally managed control plane" >&2
  exit 1
fi
fallback_dry_run="$(task --dry test:e2e:scenario:fallback 2>&1)"
fallback_cleanup_count="$(grep -Fc './test/e2e/platform/undeploy.sh' <<<"${fallback_dry_run}" || true)"
if [[ "${fallback_cleanup_count}" -ne 2 ]]; then
  echo "fallback scenario must clean before preparation and after verification" >&2
  exit 1
fi
if [[ "${fallback_dry_run}" != *'./test/e2e/cluster/istio.sh reset-if-owned'* ]]; then
  echo "fallback final cleanup must preserve externally managed Istio" >&2
  exit 1
fi

echo "public task surface contains exactly 34 supported commands: PASS"
