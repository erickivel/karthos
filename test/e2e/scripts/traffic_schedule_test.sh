#!/usr/bin/env bash

set -euo pipefail

. "$(dirname "$0")/traffic_schedule.sh"

for normal in 0 1 2 7 30; do
  for errors in 1 2 3 10; do
    schedule="$(interleaved_request_kinds "$normal" "$errors")"
    normal_count="$(printf '%s\n' "$schedule" | grep -c '^normal$' || true)"
    error_count="$(printf '%s\n' "$schedule" | grep -c '^error$' || true)"
    [[ "$normal_count" -eq "$normal" ]]
    [[ "$error_count" -eq "$errors" ]]
  done
done

echo "traffic schedule tests passed"
