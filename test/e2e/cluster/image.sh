#!/usr/bin/env bash
# Per-cell image transport. Deploys keep the same registry-qualified image tag
# in every cell while each engine owns the path that makes that tag available.

set -euo pipefail

# shellcheck disable=SC1091
. "$(dirname "${BASH_SOURCE[0]}")/common.sh"

usage() {
  cat >&2 <<EOF
usage: image.sh push_or_load <fully-qualified-tag>
example: image.sh push_or_load localhost:5001/karthos-backend:latest
EOF
  exit 2
}

split_image_reference() {
  local image="$1"
  local repository_and_tag

  case "${image}" in
    *[!A-Za-z0-9._:/-]*|*//*|*::*)
      echo "image transport: unsafe image reference: ${image}" >&2
      return 1
      ;;
  esac
  registry="${image%%/*}"
  repository_and_tag="${image#*/}"
  repository="${repository_and_tag%:*}"
  tag="${repository_and_tag##*:}"

  if [[ "${registry}" != "localhost:5001" || -z "${repository}" || "${repository}" == "${repository_and_tag}" || -z "${tag}" ]]; then
    echo "image transport: expected registry/repository:tag, got ${image}" >&2
    return 1
  fi
}

push_k3s_image() {
  local image="$1"
  local pushed_digest manifest_digest

  : "${K3S_SSH_HOST:?K3S_SSH_HOST required for CLUSTER_ENGINE=k3s}"
  split_image_reference "${image}"

  pushed_digest="$({ printf '%s\n' "${image}"; docker save "${image}"; } | ssh -o BatchMode=yes "${K3S_SSH_HOST}" 'bash -c '"'"'
set -euo pipefail
IFS= read -r image
case "$image" in
  localhost:5001/*:*) ;;
  *) echo "image transport: invalid remote image reference" >&2; exit 1 ;;
esac
case "$image" in
  *[!A-Za-z0-9._:/-]*|*//*|*::*) echo "image transport: unsafe remote image reference" >&2; exit 1 ;;
esac
docker load >&2
push_output="$(docker push "$image" 2>&1)"
printf "%s\\n" "$push_output" >&2
pushed_digest="$(printf "%s\\n" "$push_output" | sed -n "s/.*[Dd]igest: \\(sha256:[[:xdigit:]]*\\).*/\\1/p" | tail -n 1)"
if [[ ! "$pushed_digest" =~ ^sha256:[[:xdigit:]]{64}$ ]]; then
  echo "image transport: invalid remote push digest: ${pushed_digest:-missing}" >&2
  exit 1
fi
printf "%s\\n" "$pushed_digest"
'"'"'')"

  manifest_digest="$(printf '%s\n%s\n' "${repository}" "${tag}" | ssh -o BatchMode=yes "${K3S_SSH_HOST}" 'bash -c '"'"'
set -euo pipefail
IFS= read -r repository
IFS= read -r tag
if [[ -z "$repository" || "$repository" == *[!A-Za-z0-9._/-]* ]]; then
  echo "image transport: invalid remote repository" >&2
  exit 1
fi
if [[ -z "$tag" || "$tag" == *[!A-Za-z0-9._-]* ]]; then
  echo "image transport: invalid remote tag" >&2
  exit 1
fi
curl -fsSI \
  -H "Accept: application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json" \
  "http://127.0.0.1:5001/v2/${repository}/manifests/${tag}"
'"'"'' | awk 'tolower($1) == "docker-content-digest:" { gsub("\\r", "", $2); print $2; exit }')"

  if [[ ! "${manifest_digest}" =~ ^sha256:[[:xdigit:]]{64}$ ]]; then
    echo "image transport: invalid remote registry manifest digest: ${manifest_digest:-missing} for ${image}" >&2
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
