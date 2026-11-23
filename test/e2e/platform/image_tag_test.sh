#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
RESOLVER="${REPO_ROOT}/test/e2e/platform/image-tag.sh"

clean_tag="$(KARTHOS_GIT_SHA_OVERRIDE=abcdef123456 KARTHOS_GIT_DIRTY_OVERRIDE=false "${RESOLVER}")"
[[ "${clean_tag}" == "abcdef1" ]]

dirty_tag="$(
  KARTHOS_GIT_SHA_OVERRIDE=abcdef123456 \
  KARTHOS_GIT_DIRTY_OVERRIDE=true \
  KARTHOS_DIRTY_TIMESTAMP_OVERRIDE=20261001T143012Z \
  "${RESOLVER}"
)"
[[ "${dirty_tag}" == "abcdef1-dirty-20261001T143012Z" ]]

echo "platform image tags identify clean and dirty working trees: PASS"
