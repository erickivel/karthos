#!/usr/bin/env bash

set -euo pipefail

task test:e2e:scenario:fallback

python3 - <<'PY'
import json
from pathlib import Path

profile = json.loads(Path("bin/e2e/profile.json").read_text())
expected = json.loads(Path("test/e2e/scenarios/no-telemetry/expected-profile.json").read_text())

capability = next(c for c in profile["capabilities"] if c["source_id"] == expected["source_id"])
assert capability["status"] == expected["status"]
assert capability.get("evidence", {}).get("prometheus_targets", []) == []
PY
