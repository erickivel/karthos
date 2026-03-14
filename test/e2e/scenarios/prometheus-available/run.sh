#!/usr/bin/env bash

set -euo pipefail

task test:e2e:scenario:prometheus

python3 - <<'PY'
import json
from pathlib import Path

profile = json.loads(Path("bin/e2e/profile.json").read_text())
expected = json.loads(Path("test/e2e/scenarios/prometheus-available/expected-profile.json").read_text())

assert profile["recommended_receivers"] == expected["recommended_receivers"]
source = profile["sources"][0]
assert source["type"] == expected["source_expectations"]["type"]
assert source["present"] is expected["source_expectations"]["present"]
assert len(source["prometheus_targets"]) >= 1
PY
