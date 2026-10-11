#!/usr/bin/env bash

set -euo pipefail

NAMESPACE="${KIND_NAMESPACE:-karthos-system}"
RELEASE="${HELM_RELEASE_NAME:-karthos}"

helm status "${RELEASE}" -n "${NAMESPACE}"
kubectl rollout status "statefulset/${RELEASE}-postgresql" -n "${NAMESPACE}" --timeout=180s
kubectl wait --for=condition=ready pod -l "app.kubernetes.io/name=victoria-metrics-single,app.kubernetes.io/instance=${RELEASE}" -n "${NAMESPACE}" --timeout=180s
kubectl rollout status "deployment/${RELEASE}-backend" -n "${NAMESPACE}" --timeout=120s
kubectl rollout status "deployment/${RELEASE}-frontend" -n "${NAMESPACE}" --timeout=120s
kubectl rollout status "deployment/${RELEASE}-gateway" -n "${NAMESPACE}" --timeout=120s
kubectl rollout status "daemonset/${RELEASE}-agent" -n "${NAMESPACE}" --timeout=120s

registry_digest() {
  local repository="$1"
  local url="http://127.0.0.1:5001/v2/${repository}/manifests/${IMAGE_TAG}"
  local headers
  if [[ "${CLUSTER_ENGINE}" == "k3s" ]]; then
    headers="$(
      printf '%s\n' "${url}" |
        ssh -o BatchMode=yes "${K3S_SSH_HOST}" \
          'IFS= read -r url
           curl -fsSI \
             -H "Accept: application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json" \
             "$url"'
    )"
  else
    headers="$(curl -fsSI -H 'Accept: application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json' "${url}")"
  fi
  printf '%s\n' "${headers}" | awk 'tolower($1) == "docker-content-digest:" { gsub("\\r", "", $2); print $2; exit }'
}

verify_image() {
  local component="$1"
  local repository="karthos-${component}"
  local expected_image="${REGISTRY_HOST}/${repository}:${IMAGE_TAG}"
  local configured_image running_image_id digest
  configured_image="$(kubectl get "deployment/${RELEASE}-${component}" -n "${NAMESPACE}" -o "jsonpath={.spec.template.spec.containers[?(@.name=='${component}')].image}")"
  [[ "${configured_image}" == "${expected_image}" ]] || {
    echo "${component} deployment image mismatch: expected ${expected_image}, got ${configured_image}" >&2
    return 1
  }
  running_image_id="$(kubectl get pods -n "${NAMESPACE}" -l "app.kubernetes.io/component=${component}" -o "jsonpath={.items[0].status.containerStatuses[?(@.name=='${component}')].imageID}")"
  digest="$(registry_digest "${repository}")"
  [[ "${digest}" =~ ^sha256:[[:xdigit:]]{64}$ ]] || {
    echo "invalid registry digest for ${expected_image}: ${digest:-missing}" >&2
    return 1
  }
  [[ "${running_image_id}" == *@"${digest}" ]] || {
    echo "${component} running digest mismatch: registry ${digest}, pod ${running_image_id}" >&2
    return 1
  }
  printf '%s image=%s digest=%s\n' "${component}" "${expected_image}" "${digest}"
}

verify_image backend
verify_image frontend
