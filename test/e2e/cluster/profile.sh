#!/usr/bin/env bash
# Source me. Resolves the active cluster profile and exports its variables.
#
# Resolution order:
#   1. Load .env (if present) so CLUSTER_PROFILE can be set there.
#   2. Default CLUSTER_PROFILE to "oksg-test".
#   3. Load .env.d/${CLUSTER_PROFILE}.env (required).
#   4. Apply final defaults.
#
# Must be sourced from the repo root (Taskfile.test.yml `sh:` invocations
# run there; standalone scripts source this only after `cd` to repo root or
# via an absolute path).

__OKSG_REPO_ROOT="${PWD}"

if [ -f "${__OKSG_REPO_ROOT}/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "${__OKSG_REPO_ROOT}/.env"
  set +a
fi

: "${CLUSTER_PROFILE:=oksg-test}"
__OKSG_PROFILE_FILE="${__OKSG_REPO_ROOT}/.env.d/${CLUSTER_PROFILE}.env"

if [ ! -f "${__OKSG_PROFILE_FILE}" ]; then
  echo "cluster profile not found: ${__OKSG_PROFILE_FILE}" >&2
  echo "set CLUSTER_PROFILE in .env (default: oksg-test) and create the matching .env.d/<name>.env" >&2
  return 1 2>/dev/null || exit 1
fi

set -a
# shellcheck disable=SC1090
. "${__OKSG_PROFILE_FILE}"
set +a

# A caller-selected tag must win over a profile's convenient default. This is
# used for uncommitted or otherwise non-default images that must not reuse a
# mutable evaluation tag on a remote registry.
if [ -n "${OKSG_IMAGE_TAG_OVERRIDE:-}" ]; then
  IMAGE_TAG="${OKSG_IMAGE_TAG_OVERRIDE}"
fi

: "${CLUSTER_ENGINE:=kind}"
: "${CLUSTER_LOCATION:=local}"
REGISTRY_HOST="localhost:5001"
: "${IMAGE_TAG:=latest}"
: "${IMAGE_PLATFORM:=}"
: "${KUBECONFIG:=${HOME}/.kube/config}"

export CLUSTER_PROFILE CLUSTER_ENGINE CLUSTER_LOCATION REGISTRY_HOST IMAGE_TAG IMAGE_PLATFORM KUBECONFIG OKSG_IMAGE_TAG_OVERRIDE
