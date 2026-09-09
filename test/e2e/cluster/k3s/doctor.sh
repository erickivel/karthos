#!/usr/bin/env bash
# Pre-flight checker for the k3s+ssh cell.
# Exits 0 only if every check passes. Each step prints OK/FAIL on its own line.

set -uo pipefail

# shellcheck disable=SC1091
. "$(dirname "${BASH_SOURCE[0]}")/../common.sh"

: "${K3S_SSH_HOST:?K3S_SSH_HOST required (check .env.d/${CLUSTER_PROFILE}.env)}"
: "${K3S_SERVER_IP:?K3S_SERVER_IP required (check .env.d/${CLUSTER_PROFILE}.env)}"
: "${K3S_API_PORT:=6443}"

FAILED=0
pass() { printf '  [ ok ] %s\n' "$1"; }
fail() { printf '  [FAIL] %s\n' "$1"; FAILED=1; }
warn() { printf '  [warn] %s\n' "$1"; }

SSH="ssh -o BatchMode=yes -o ConnectTimeout=5 ${K3S_SSH_HOST}"

echo "== profile =="
echo "  CLUSTER_PROFILE=${CLUSTER_PROFILE}"
echo "  CLUSTER_ENGINE=${CLUSTER_ENGINE}"
echo "  CLUSTER_LOCATION=${CLUSTER_LOCATION}"
echo "  K3S_SSH_HOST=${K3S_SSH_HOST}"
echo "  K3S_SERVER_IP=${K3S_SERVER_IP}:${K3S_API_PORT}"
echo "  REGISTRY_HOST=${REGISTRY_HOST}"
echo "  KUBECONFIG=${KUBECONFIG}"

echo "== checks =="

if ${SSH} true 2>/dev/null; then
  pass "ssh ${K3S_SSH_HOST} reachable"
else
  fail "ssh ${K3S_SSH_HOST} not reachable; check your SSH config and keys"
fi

if ${SSH} "systemctl is-active --quiet k3s" 2>/dev/null; then
  pass "k3s service active on the server"
else
  fail "k3s service not active on the server (try: ssh ${K3S_SSH_HOST} 'sudo systemctl status k3s')"
fi

if ${SSH} "test -f /etc/rancher/k3s/registries.yaml && grep -q 'localhost:5001' /etc/rancher/k3s/registries.yaml" 2>/dev/null; then
  pass "/etc/rancher/k3s/registries.yaml present and mirrors localhost:5001"
else
  fail "registries.yaml missing or does not mirror localhost:5001 (run: task test:e2e:setup)"
fi

if ${SSH} "docker inspect -f '{{.State.Running}}' karthos-registry 2>/dev/null | grep -qx true" 2>/dev/null; then
  pass "karthos-registry container running on the server"
else
  fail "karthos-registry container not running (run: task test:e2e:setup)"
fi

if ${SSH} "curl -fsS --max-time 5 http://127.0.0.1:5001/v2/" >/dev/null 2>&1; then
  pass "remote registry reachable from the k3s server at 127.0.0.1:5001"
else
  fail "remote registry is not reachable from the k3s server at 127.0.0.1:5001 (run: task test:e2e:setup)"
fi

if kubectl --kubeconfig "${KUBECONFIG}" get --raw=/readyz >/dev/null 2>&1; then
  pass "kubectl reachable via ${KUBECONFIG}"
else
  fail "kubectl unreachable via ${KUBECONFIG}; check K3S_SERVER_IP and TLS SAN (see warn below)"
fi

# TLS SAN probe is informational: if SERVER_IP isn't in the cert, kubectl
# will fail with x509 errors. Print the remediation regardless of whether
# the previous kubectl call succeeded so users know how to extend the cert
# if/when they change SERVER_IP.
CERT_SANS="$(echo | openssl s_client -connect "${K3S_SERVER_IP}:${K3S_API_PORT}" -showcerts 2>/dev/null \
              | openssl x509 -noout -text 2>/dev/null \
              | awk '/Subject Alternative Name/{getline; print}' || true)"
if [ -n "${CERT_SANS}" ] && echo "${CERT_SANS}" | grep -q "${K3S_SERVER_IP}"; then
  pass "TLS cert includes SAN for ${K3S_SERVER_IP}"
else
  warn "TLS cert may not include a SAN for ${K3S_SERVER_IP}."
  warn "  if kubectl above failed, on ${K3S_SSH_HOST} edit the k3s service to add:"
  warn "    --tls-san=${K3S_SERVER_IP}"
  warn "  then: sudo systemctl restart k3s, and re-run: task test:e2e:setup"
fi

echo "============"
if [ "${FAILED}" -eq 0 ]; then
  echo "all required checks passed"
  exit 0
else
  echo "one or more checks failed"
  exit 1
fi
