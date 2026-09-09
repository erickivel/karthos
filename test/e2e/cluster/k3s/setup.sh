#!/usr/bin/env bash
# Prepare a remote k3s host for Karthos e2e use.
#
# Assumes k3s is ALREADY installed and reachable via `ssh $K3S_SSH_HOST`.
# This script:
#   1. Verifies SSH + kubectl access on the server.
#   2. Ensures an "karthos-registry" container is running on the server,
#      bound to 127.0.0.1:5001.
#   3. Installs /etc/rancher/k3s/registries.yaml mirroring localhost:5001
#      to that container, and restarts k3s only if the file changed.
#   4. Fetches /etc/rancher/k3s/k3s.yaml, rewrites the server URL to
#      https://${K3S_SERVER_IP}:${K3S_API_PORT}, and writes it to $KUBECONFIG.
#
# Never installs k3s. Never touches k3s on teardown.

set -euo pipefail

# shellcheck disable=SC1091
. "$(dirname "${BASH_SOURCE[0]}")/../common.sh"

require() {
  if [ -z "${!1:-}" ]; then
    echo "k3s setup: $1 must be set (check .env.d/${CLUSTER_PROFILE}.env)" >&2
    exit 1
  fi
}

require K3S_SSH_HOST
require K3S_SERVER_IP
: "${K3S_API_PORT:=6443}"

SSH="ssh -o BatchMode=yes ${K3S_SSH_HOST}"

echo "==> verifying SSH access and k3s service on ${K3S_SSH_HOST}"
${SSH} true \
  || { echo "ssh to ${K3S_SSH_HOST} failed; check ~/.ssh/config and that your key is loaded" >&2; exit 1; }
${SSH} "systemctl is-active --quiet k3s" \
  || { echo "k3s service is not active on ${K3S_SSH_HOST} (try: ssh ${K3S_SSH_HOST} 'sudo systemctl status k3s')" >&2; exit 1; }

echo "==> ensuring karthos-registry container on ${K3S_SSH_HOST}"
${SSH} bash -se <<EOF
set -euo pipefail
if docker inspect -f '{{.State.Running}}' karthos-registry 2>/dev/null | grep -qx true; then
  echo "registry already running"
else
  docker rm -f karthos-registry 2>/dev/null || true
  docker run -d --restart=always \
    -p 127.0.0.1:5001:5000 \
    --name karthos-registry \
    registry:2
fi
EOF

echo "==> rendering registries.yaml"
TMP="$(mktemp -t karthos-registries.XXXXXX)"
LOCAL_SERVER_SCRIPT="$(mktemp -t karthos-server-setup.XXXXXX)"
trap 'rm -f "${TMP}" "${LOCAL_SERVER_SCRIPT}"' EXIT
cat >"${TMP}" <<EOF
mirrors:
  "localhost:5001":
    endpoint:
      - "http://127.0.0.1:5001"
EOF

# Allocate a per-run tmpdir on the server so concurrent invocations from
# different laptops sharing one host cannot collide on /tmp paths.
REMOTE_TMPDIR="$(${SSH} 'mktemp -d -t karthos-setup.XXXXXX')"
REMOTE_TMP="${REMOTE_TMPDIR}/registries.yaml"
REMOTE_KUBECONFIG="${REMOTE_TMPDIR}/kubeconfig.yaml"
REMOTE_SCRIPT="${REMOTE_TMPDIR}/server-setup.sh"
scp -q "${TMP}" "${K3S_SSH_HOST}:${REMOTE_TMP}"
cat >"${LOCAL_SERVER_SCRIPT}" <<'EOS'
#!/usr/bin/env bash
set -euo pipefail
REGISTRIES_SRC="$1"
KUBECONFIG_DST="$2"
SUDO_USER_NAME="$3"

DST="/etc/rancher/k3s/registries.yaml"
if sudo test -f "$DST" && sudo cmp -s "$REGISTRIES_SRC" "$DST"; then
  echo "registries.yaml unchanged"
else
  sudo install -m 0644 "$REGISTRIES_SRC" "$DST"
  echo "registries.yaml installed; restarting k3s"
  sudo systemctl restart k3s
fi
rm -f "$REGISTRIES_SRC"

# Emit a user-readable copy of the kubeconfig so the dev side can scp it
# back without needing sudo over a non-TTY ssh session.
sudo install -m 0644 -o "$SUDO_USER_NAME" /etc/rancher/k3s/k3s.yaml "$KUBECONFIG_DST"
EOS

scp -q "${LOCAL_SERVER_SCRIPT}" "${K3S_SSH_HOST}:${REMOTE_SCRIPT}"

echo "==> installing registries.yaml + exporting kubeconfig (one sudo prompt)"
ssh -t "${K3S_SSH_HOST}" \
  "bash '${REMOTE_SCRIPT}' '${REMOTE_TMP}' '${REMOTE_KUBECONFIG}' \"\$USER\""

echo "==> fetching kubeconfig to ${KUBECONFIG}"
KUBECONFIG_DIR="$(dirname "${KUBECONFIG}")"
mkdir -p "${KUBECONFIG_DIR}"
LOCAL_KUBECONFIG_TMP="$(mktemp -t karthos-k3s-kubeconfig.XXXXXX)"
scp -q "${K3S_SSH_HOST}:${REMOTE_KUBECONFIG}" "${LOCAL_KUBECONFIG_TMP}"
${SSH} "rm -rf '${REMOTE_TMPDIR}'"
sed "s#https://127.0.0.1:6443#https://${K3S_SERVER_IP}:${K3S_API_PORT}#" "${LOCAL_KUBECONFIG_TMP}" >"${KUBECONFIG}"
chmod 600 "${KUBECONFIG}"
rm -f "${LOCAL_KUBECONFIG_TMP}"

cat <<EOF

k3s+ssh cell ready.

Next steps:
  export KUBECONFIG=${KUBECONFIG}
  task test:cluster:doctor    # validate the cell
  # Optional: task test:cluster:tunnel  # inspect the remote registry from the laptop

EOF
