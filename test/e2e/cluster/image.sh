#!/usr/bin/env bash
# Per-cell image transport.
#
# Today this is a thin wrapper around `docker push` for both supported
# cells — the only `case` arm is `push_or_load`, which always pushes:
#   - kind+local: docker push -> local "registry:2" container that kind's
#     containerd mirrors at localhost:5001.
#   - k3s+ssh:   docker push -> SSH tunnel -> remote "registry:2" container.
#     k3s containerd mirrors localhost:5001 -> http://127.0.0.1:5001 via
#     /etc/rancher/k3s/registries.yaml.
#
# The wrapper exists so deploy.sh never needs to know which transport a
# cell uses; a future kind variant can swap in `kind load docker-image`
# here (paired with flipping pullPolicy=Always) without touching deploy.sh.

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

tag_is_resolvable() {
  local registry="$1"
  local repository="$2"
  local tag="$3"

  curl -fsSI \
    -H 'Accept: application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json' \
    "http://${registry}/v2/${repository}/manifests/${tag}" >/dev/null
}

ensure_remote_tag() {
  local image="$1"
  local registry repository_and_tag repository tag digest_ref digest manifest

  registry="${image%%/*}"
  repository_and_tag="${image#*/}"
  repository="${repository_and_tag%:*}"
  tag="${repository_and_tag##*:}"

  if tag_is_resolvable "$registry" "$repository" "$tag"; then
    return 0
  fi

  digest_ref="$(docker image inspect --format '{{index .RepoDigests 0}}' "$image")"
  digest="${digest_ref##*@}"
  if [[ -z "$digest" || "$digest" == "$digest_ref" ]]; then
    echo "image transport: pushed $image but could not determine its manifest digest" >&2
    return 1
  fi

  manifest="$(mktemp -t oksg-registry-manifest.XXXXXX)"
  trap 'rm -f "$manifest"' RETURN
  curl -fsS \
    -H 'Accept: application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json' \
    "http://${registry}/v2/${repository}/manifests/${digest}" \
    -o "$manifest"
  curl -fsS -X PUT \
    -H 'Content-Type: application/vnd.docker.distribution.manifest.v2+json' \
    --data-binary "@$manifest" \
    "http://${registry}/v2/${repository}/manifests/${tag}" >/dev/null
  tag_is_resolvable "$registry" "$repository" "$tag"
}

case "${1:-}" in
  push_or_load)
    shift
    [ $# -eq 1 ] || usage
    docker push "$1"
    ensure_remote_tag "$1"
    ;;
  *)
    usage
    ;;
esac
