#!/usr/bin/env bash

# Print an evenly distributed sequence containing exactly the requested
# numbers of normal and error requests. The cumulative target is a discrete
# line (Bresenham-style), so no division interval can overshoot.
interleaved_request_kinds() {
  local normal="$1"
  local errors="$2"
  local total=$((normal + errors))
  local errors_sent=0
  local i target_errors

  if ((total <= 0)); then
    return
  fi

  for ((i = 1; i <= total; i++)); do
    target_errors=$((i * errors / total))
    if ((target_errors > errors_sent)); then
      printf 'error\n'
      errors_sent=$((errors_sent + 1))
    else
      printf 'normal\n'
    fi
  done
}
