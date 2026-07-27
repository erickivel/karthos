#!/usr/bin/env bash
# Per-cell image transport. Deploys keep the same registry-qualified image tag
# in every cell while each engine owns the path that makes that tag available.

set -euo pipefail

# shellcheck disable=SC1091
. "$(dirname "${BASH_SOURCE[0]}")/common.sh"

usage() {
  cat >&2 <<EOF
usage: image.sh push_or_load <fully-qualified-tag>
example: image.sh push_or_load localhost:5001/oksg-backend:latest
EOF
  exit 2
}

split_image_reference() {
  local image="$1"
  local repository_and_tag

  registry="${image%%/*}"
  repository_and_tag="${image#*/}"
  repository="${repository_and_tag%:*}"
  tag="${repository_and_tag##*:}"

  if [[ "${registry}" == "${image}" || "${repository}" == "${repository_and_tag}" ]]; then
    echo "image transport: expected registry/repository:tag, got ${image}" >&2
    return 1
  fi
}

push_k3s_image() {
  local image="$1"
  local pushed_digest manifest_digest

  : "${K3S_SSH_HOST:?K3S_SSH_HOST required for CLUSTER_ENGINE=k3s}"
  : "${K3S_REGISTRY_PORT:=5001}"
  split_image_reference "${image}"

  pushed_digest="$(docker save "${image}" | ssh -o BatchMode=yes "${K3S_SSH_HOST}" bash -c '
set -euo pipefail
image="$1"
docker load >&2
push_output="$(docker push "$image" 2>&1)"
printf "%s\\n" "$push_output" >&2
pushed_digest="$(printf "%s\\n" "$push_output" | sed -n "s/.*[Dd]igest: \\(sha256:[[:xdigit:]]*\\).*/\\1/p" | tail -n 1)"
if [[ -z "$pushed_digest" ]]; then
  echo "image transport: remote docker push did not print a manifest digest" >&2
  exit 1
fi
printf "%s\\n" "$pushed_digest"
' -- "${image}")"

  manifest_digest="$(ssh -o BatchMode=yes "${K3S_SSH_HOST}" bash -c '
set -euo pipefail
repository="$1"
tag="$2"
registry_port="$3"
curl -fsSI \
  -H "Accept: application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json" \
  "http://127.0.0.1:${registry_port}/v2/${repository}/manifests/${tag}"
' -- "${repository}" "${tag}" "${K3S_REGISTRY_PORT}" | awk 'tolower($1) == "docker-content-digest:" { gsub("\\r", "", $2); print $2; exit }')"

  if [[ -z "${manifest_digest}" ]]; then
    echo "image transport: remote registry did not return Docker-Content-Digest for ${image}" >&2
    return 1
  fi
  if [[ "${pushed_digest}" != "${manifest_digest}" ]]; then
    echo "image transport: remote push digest ${pushed_digest} does not match registry manifest digest ${manifest_digest} for ${image}" >&2
    return 1
  fi
}

case "${1:-}" in
  push_or_load)
    shift
    [ $# -eq 1 ] || usage
    case "${CLUSTER_ENGINE}" in
      kind)
        docker push "$1"
        ;;
      k3s)
        push_k3s_image "$1"
        ;;
      *)
        echo "unsupported CLUSTER_ENGINE: ${CLUSTER_ENGINE}" >&2
        exit 1
        ;;
    esac
    ;;
  *)
    usage
    ;;
esac
