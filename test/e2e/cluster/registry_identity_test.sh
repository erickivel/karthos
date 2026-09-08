#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t oksg-registry-identity-test.XXXXXX)"
cleanup() {
  local status=$?
  rm -rf "${TMPDIR_TEST}"
  exit "${status}"
}
trap cleanup EXIT

mkdir -p "${TMPDIR_TEST}/bin" "${TMPDIR_TEST}/kind" "${TMPDIR_TEST}/k3s/test/e2e" "${TMPDIR_TEST}/k3s/.env.d"
cp "${REPO_ROOT}/test/e2e/kind/setup.sh" "${TMPDIR_TEST}/kind/setup.sh"
cp "${REPO_ROOT}/test/e2e/kind/cluster-config.yaml" "${TMPDIR_TEST}/kind/cluster-config.yaml"
cp -R "${REPO_ROOT}/test/e2e/cluster" "${TMPDIR_TEST}/k3s/test/e2e/cluster"

cat >"${TMPDIR_TEST}/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >>"${REGISTRY_TEST_LOG}"
case "${1:-}" in
  inspect)
    if [[ "$*" == *'NetworkSettings'* ]]; then
      printf 'null\n'
    else
      printf 'false\n'
    fi
    ;;
esac
EOF
cat >"${TMPDIR_TEST}/bin/kind" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'kind %s\n' "$*" >>"${REGISTRY_TEST_LOG}"
EOF
cat >"${TMPDIR_TEST}/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'kubectl %s\n' "$*" >>"${REGISTRY_TEST_LOG}"
if [[ "${1:-}" == "apply" ]]; then
  cat >>"${REGISTRY_TEST_MANIFEST}"
fi
EOF
cat >"${TMPDIR_TEST}/bin/ssh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'ssh %s\n' "$*" >>"${REGISTRY_TEST_LOG}"
EOF
cat >"${TMPDIR_TEST}/bin/openssl" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "${TMPDIR_TEST}/bin/"*

export PATH="${TMPDIR_TEST}/bin:${PATH}"
export REGISTRY_TEST_LOG="${TMPDIR_TEST}/commands.log"
export REGISTRY_TEST_MANIFEST="${TMPDIR_TEST}/registry-config.yaml"

bash "${TMPDIR_TEST}/kind/setup.sh" fixture-cluster
grep -Fx 'docker run -d --restart=always -p 127.0.0.1:5001:5000 --name oksg-kind-registry registry:2' "${REGISTRY_TEST_LOG}"
grep -Fqx '    host: "localhost:5001"' "${REGISTRY_TEST_MANIFEST}"

printf '%s\n' 'CLUSTER_PROFILE=fixture' >"${TMPDIR_TEST}/k3s/.env"
cat >"${TMPDIR_TEST}/k3s/.env.d/fixture.env" <<'EOF'
CLUSTER_ENGINE=k3s
CLUSTER_LOCATION=ssh
K3S_SSH_HOST=fixture-host
K3S_SERVER_IP=127.0.0.1
REGISTRY_HOST=localhost:5001
KUBECONFIG=/tmp/fixture-kubeconfig
EOF
: >"${REGISTRY_TEST_LOG}"
(
  cd "${TMPDIR_TEST}/k3s"
  bash test/e2e/cluster/k3s/doctor.sh
)
grep -Fqx 'ssh -o BatchMode=yes -o ConnectTimeout=5 fixture-host curl -fsS --max-time 5 http://127.0.0.1:5001/v2/' "${REGISTRY_TEST_LOG}"

echo "kind and k3s use the fixed localhost:5001 registry identity: PASS"
