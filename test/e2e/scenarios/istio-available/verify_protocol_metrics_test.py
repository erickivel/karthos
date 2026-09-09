#!/usr/bin/env python3

import importlib.util
import unittest
from pathlib import Path
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location(
    "verify_protocol_metrics",
    Path(__file__).with_name("verify_protocol_metrics.py"),
)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class VerifyProtocolMetricsTest(unittest.TestCase):
    def test_api_polling_uses_collector_propagation_budget(self):
        self.assertEqual(120, MODULE.API_POLL_TIMEOUT_SECONDS)

    def test_api_edge_total_allows_no_data(self):
        with patch.object(MODULE, "api_payload", return_value={"samples": []}):
            payload, total = MODULE.api_edge_total("metric", "increase", "count")
        self.assertEqual({"samples": []}, payload)
        self.assertIsNone(total)

    def test_raw_values_require_destination_identity(self):
        metric = MODULE.METRICS["grpc_requests"]["raw"]
        lines = [
            f'{metric}{{reporter="destination",source_workload="mesh-protocol-client",destination_workload="other"}} 99',
            f'{metric}{{reporter="destination",source_workload="mesh-protocol-client",destination_workload="mesh-protocol-server"}} 25',
        ]
        values = MODULE.raw_values_from_lines(lines)
        self.assertEqual(25, values["grpc_requests"])


if __name__ == "__main__":
    unittest.main()
