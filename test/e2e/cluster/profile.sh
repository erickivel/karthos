#!/usr/bin/env bash
# Source me. Resolves the active cluster profile and exports its variables.
#
# Resolution order:
#   1. Load .env (if present) so CLUSTER_PROFILE can be set there.
#   2. Default CLUSTER_PROFILE to "karthos-test".
#   3. Load .env.d/${CLUSTER_PROFILE}.env (required).
#   4. Apply final defaults.
#
# Must be sourced from the repo root (Taskfile.test.yml `sh:` invocations
# run there; standalone scripts source this only after `cd` to repo root or
# via an absolute path).

__KARTHOS_REPO_ROOT="${PWD}"
if [ -f "${__KARTHOS_REPO_ROOT}/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "${__KARTHOS_REPO_ROOT}/.env"
  set +a
fi

if [ -n "${KARTHOS_CLUSTER_PROFILE_OVERRIDE:-}" ]; then
  CLUSTER_PROFILE="${KARTHOS_CLUSTER_PROFILE_OVERRIDE}"
fi

: "${CLUSTER_PROFILE:=karthos-test}"
if [ -n "${KARTHOS_CLUSTER_PROFILE_FILE_OVERRIDE:-}" ]; then
  __KARTHOS_PROFILE_FILE="${KARTHOS_CLUSTER_PROFILE_FILE_OVERRIDE}"
else
  __KARTHOS_PROFILE_FILE="${__KARTHOS_REPO_ROOT}/.env.d/${CLUSTER_PROFILE}.env"
fi

if [ ! -f "${__KARTHOS_PROFILE_FILE}" ]; then
  echo "cluster profile not found: ${__KARTHOS_PROFILE_FILE}" >&2
  echo "set CLUSTER_PROFILE in .env (default: karthos-test) and create the matching .env.d/<name>.env" >&2
  return 1 2>/dev/null || exit 1
fi

set -a
# shellcheck disable=SC1090
. "${__KARTHOS_PROFILE_FILE}"
set +a

# A caller-selected tag must win over a profile's convenient default. This is
# used for uncommitted or otherwise non-default images that must not reuse a
# mutable evaluation tag on a remote registry.
if [ -n "${KARTHOS_IMAGE_TAG_OVERRIDE:-}" ]; then
  IMAGE_TAG="${KARTHOS_IMAGE_TAG_OVERRIDE}"
fi

: "${CLUSTER_ENGINE:=kind}"
: "${CLUSTER_LOCATION:=local}"
REGISTRY_HOST="localhost:5001"
: "${IMAGE_TAG:=latest}"
: "${IMAGE_PLATFORM:=}"
: "${KUBECONFIG:=${HOME}/.kube/config}"

export CLUSTER_PROFILE CLUSTER_ENGINE CLUSTER_LOCATION REGISTRY_HOST IMAGE_TAG IMAGE_PLATFORM KUBECONFIG KARTHOS_IMAGE_TAG_OVERRIDE
