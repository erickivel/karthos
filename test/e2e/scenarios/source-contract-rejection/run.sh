#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
cd "${REPO_ROOT}"

NAMESPACE="${KIND_NAMESPACE:-karthos-system}"
CONTRACT_NAME="${HELM_RELEASE_NAME:-karthos}-source-contract"
TMPDIR_TEST="$(mktemp -d -t karthos-source-contract-rejection.XXXXXX)"
BACKUP="${TMPDIR_TEST}/source-contract.json"
RESTORE="${TMPDIR_TEST}/restore-source-contract.json"
BACKEND_DEPLOYMENT="${HELM_RELEASE_NAME:-karthos}-backend"

wait_for_observed_generation() {
  local timeout="${1:-45s}"
  local generation
  generation="$(kubectl get deployment/${BACKEND_DEPLOYMENT} -n "${NAMESPACE}" -o jsonpath='{.metadata.generation}')"
  if [[ -z "${generation}" ]]; then
    echo "could not read backend deployment generation" >&2
    return 1
  fi
  kubectl wait --for=jsonpath='{.status.observedGeneration}'="${generation}" deployment/${BACKEND_DEPLOYMENT} -n "${NAMESPACE}" --timeout="${timeout}"
}

wait_for_mounted_contract() {
  local expected_fingerprint="intentionally-mismatched-fingerprint"
  local attempt
  for attempt in {1..90}; do
    if kubectl exec "deployment/${BACKEND_DEPLOYMENT}" -n "${NAMESPACE}" -- cat /etc/karthos/source-plan.json 2>/dev/null | grep -Fq "${expected_fingerprint}"; then
      return 0
    fi
    sleep 1
  done
  echo "backend Pod did not receive the mutated source contract before restart" >&2
  return 1
}

restore() {
  local original_status=$?
  local restore_status=0
  set +e
  if kubectl get configmap "${CONTRACT_NAME}" -n "${NAMESPACE}" -o json >"${RESTORE}" 2>/dev/null; then
    python3 - "${BACKUP}" "${RESTORE}" <<'PY' >/dev/null 2>&1 || true
import json
import sys

backup_path, restore_path = sys.argv[1:]
backup = json.load(open(backup_path))
restore = json.load(open(restore_path))
restore["data"] = backup["data"]
open(restore_path, "w").write(json.dumps(restore))
PY
    kubectl replace -f "${RESTORE}" >/dev/null 2>&1 || restore_status=1
  else
    restore_status=1
  fi
  task test:e2e:apply-otel >/dev/null 2>&1 || restore_status=1
  # rollout restart writes an out-of-band Pod-template annotation. Helm
  # preserves it, so remove that exact template field after the valid contract
  # is restored and Kubernetes will return to the Helm-owned revision.
  kubectl patch deployment/${BACKEND_DEPLOYMENT} -n "${NAMESPACE}" --type=json -p='[{"op":"remove","path":"/spec/template/metadata/annotations/kubectl.kubernetes.io~1restartedAt"}]' >/dev/null 2>&1 || restore_status=1
  wait_for_observed_generation 120s >/dev/null 2>&1 || restore_status=1
  kubectl rollout status deployment/${BACKEND_DEPLOYMENT} -n "${NAMESPACE}" --timeout=120s >/dev/null 2>&1 || restore_status=1
  rm -rf "${TMPDIR_TEST}"
  if [[ ${original_status} -eq 0 && ${restore_status} -ne 0 ]]; then
    exit "${restore_status}"
  fi
  exit "${original_status}"
}
trap restore EXIT

kubectl get configmap "${CONTRACT_NAME}" -n "${NAMESPACE}" -o json >"${BACKUP}"
MUTATED="${TMPDIR_TEST}/mutated-source-contract.json"
kubectl get configmap "${CONTRACT_NAME}" -n "${NAMESPACE}" -o json >"${MUTATED}"
python3 - "${MUTATED}" <<'PY'
import json
import sys
path = sys.argv[1]
data = json.load(open(path))
plan = json.loads(data["data"]["source-plan.json"])
plan["profile_fingerprint"] = "intentionally-mismatched-fingerprint"
data["data"]["source-plan.json"] = json.dumps(plan)
open(path, "w").write(json.dumps(data))
PY
kubectl replace -f "${MUTATED}"
wait_for_mounted_contract
kubectl rollout restart deployment/${BACKEND_DEPLOYMENT} -n "${NAMESPACE}"
wait_for_observed_generation
if kubectl rollout status deployment/${BACKEND_DEPLOYMENT} -n "${NAMESPACE}" --timeout=45s; then
  echo "backend accepted a fingerprint-mismatched source contract" >&2
  exit 1
fi
FAILED_POD="$(kubectl get pods -n "${NAMESPACE}" -l app.kubernetes.io/component=backend --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1:].metadata.name}')"
kubectl logs "pod/${FAILED_POD}" -n "${NAMESPACE}" --previous --tail=80 | grep -Fq "source plan profile fingerprint does not match"
echo "Source-plan fingerprint mismatch is fail-closed: PASS"
