#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
NAMESPACE="${KIND_NAMESPACE:-oksg-system}"
TMPDIR_TEST="$(mktemp -d -t oksg-source-contract-rejection.XXXXXX)"
BACKUP="${TMPDIR_TEST}/source-contract.yaml"
restore() {
  kubectl apply -f "${BACKUP}" >/dev/null 2>&1 || true
  kubectl rollout restart deployment/oksg-backend -n "${NAMESPACE}" >/dev/null 2>&1 || true
  kubectl rollout status deployment/oksg-backend -n "${NAMESPACE}" --timeout=120s >/dev/null 2>&1 || true
  rm -rf "${TMPDIR_TEST}"
}
trap restore EXIT

kubectl get configmap oksg-source-contract -n "${NAMESPACE}" -o yaml >"${BACKUP}"
PROFILE="${TMPDIR_TEST}/capability-profile.json"
PLAN="${TMPDIR_TEST}/source-plan.json"
kubectl get configmap oksg-source-contract -n "${NAMESPACE}" -o jsonpath='{.data.capability-profile\.json}' >"${PROFILE}"
kubectl get configmap oksg-source-contract -n "${NAMESPACE}" -o jsonpath='{.data.source-plan\.json}' >"${PLAN}"
python3 - "${PLAN}" <<'PY'
import json
import sys
path = sys.argv[1]
data = json.load(open(path))
data["profile_fingerprint"] = "intentionally-mismatched-fingerprint"
open(path, "w").write(json.dumps(data))
PY
kubectl create configmap oksg-source-contract -n "${NAMESPACE}" \
  --from-file=capability-profile.json="${PROFILE}" \
  --from-file=source-plan.json="${PLAN}" \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl rollout restart deployment/oksg-backend -n "${NAMESPACE}"
if kubectl rollout status deployment/oksg-backend -n "${NAMESPACE}" --timeout=45s; then
  echo "backend accepted a fingerprint-mismatched source contract" >&2
  exit 1
fi
kubectl logs deployment/oksg-backend -n "${NAMESPACE}" --tail=80 | grep -Fq "source plan profile fingerprint does not match"
echo "Source-plan fingerprint mismatch is fail-closed: PASS"
