#!/usr/bin/env bash

set -euo pipefail

task test:e2e:scenario:fallback

python3 - <<'PY'
import json
from pathlib import Path

profile = json.loads(Path("bin/e2e/profile.json").read_text())
expected = json.loads(Path("test/e2e/scenarios/no-telemetry/expected-profile.json").read_text())

assert profile["recommended_receivers"] == expected["recommended_receivers"]
source = profile["sources"][0]
assert source["type"] == expected["source_expectations"]["type"]
assert source["present"] is expected["source_expectations"]["present"]
assert source.get("prometheus_targets", []) == []
PY
