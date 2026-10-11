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

echo "public task surface contains exactly 34 supported commands: PASS"
