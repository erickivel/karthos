#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${KIND_NAMESPACE:-oksg-system}"
CONTRACT_NAME="${HELM_RELEASE_NAME:-oksg-otel}-source-contract"
TMPDIR_TEST="$(mktemp -d -t oksg-source-contract-rejection.XXXXXX)"
BACKUP="${TMPDIR_TEST}/source-contract.json"
RESTORE="${TMPDIR_TEST}/restore-source-contract.json"
wait_for_observed_generation() {
  local timeout="${1:-45s}"
  local generation
  generation="$(kubectl get deployment/oksg-backend -n "${NAMESPACE}" -o jsonpath='{.metadata.generation}')"
  if [[ -z "${generation}" ]]; then
    echo "could not read backend deployment generation" >&2
    return 1
  fi
  kubectl wait --for=jsonpath='{.status.observedGeneration}'="${generation}" deployment/oksg-backend -n "${NAMESPACE}" --timeout="${timeout}"
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
  # rollout restart adds an out-of-band Pod-template annotation. Helm preserves
  # annotations it does not own, so remove it after Helm has restored the
  # ConfigMap; that makes Kubernetes create a fresh Pod from the valid contract
  # instead of continuing to reconcile the failed ReplicaSet.
  kubectl annotate deployment/oksg-backend -n "${NAMESPACE}" kubectl.kubernetes.io/restartedAt- >/dev/null 2>&1 || restore_status=1
  wait_for_observed_generation 120s >/dev/null 2>&1 || restore_status=1
  kubectl rollout status deployment/oksg-backend -n "${NAMESPACE}" --timeout=120s >/dev/null 2>&1 || restore_status=1
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
kubectl rollout restart deployment/oksg-backend -n "${NAMESPACE}"
wait_for_observed_generation
if kubectl rollout status deployment/oksg-backend -n "${NAMESPACE}" --timeout=45s; then
  echo "backend accepted a fingerprint-mismatched source contract" >&2
  exit 1
fi
FAILED_POD="$(kubectl get pods -n "${NAMESPACE}" -l app.kubernetes.io/component=backend --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1:].metadata.name}')"
kubectl logs "pod/${FAILED_POD}" -n "${NAMESPACE}" --previous --tail=80 | grep -Fq "source plan profile fingerprint does not match"
echo "Source-plan fingerprint mismatch is fail-closed: PASS"
