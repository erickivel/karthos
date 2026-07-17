#!/usr/bin/env python3

import argparse
import json
import math
import re
import time
import urllib.parse
import urllib.request
from pathlib import Path

RAW_URL = "http://localhost:19020/stats/prometheus"
VM_URL = "http://localhost:18428/api/v1/query"
API_URL = "http://localhost:18081/api/v1/metrics"
BASELINE_PATH = Path("bin/e2e/istio-protocol-baseline.json")
# Metric delivery crosses the destination sidecar, Prometheus receiver, agent,
# gateway, and backend before the API can evaluate a one-minute window.
API_POLL_TIMEOUT_SECONDS = 120

METRICS = {
    "grpc_requests": {
        "raw": "istio_request_messages_total",
        "canonical": "servicegraph.grpc.request.message.count",
        "vm": "servicegraph_grpc_request_message_count",
        "increase": 25.0,
        "rate": 25.0 / 60.0,
        "base_unit": "count",
        "rate_unit": "per_second",
    },
    "grpc_responses": {
        "raw": "istio_response_messages_total",
        "canonical": "servicegraph.grpc.response.message.count",
        "vm": "servicegraph_grpc_response_message_count",
        "increase": 25.0,
        "rate": 25.0 / 60.0,
        "base_unit": "count",
        "rate_unit": "per_second",
    },
    "tcp_opened": {
        "raw": "istio_tcp_connections_opened_total",
        "canonical": "servicegraph.tcp.connection.open.count",
        "vm": "servicegraph_tcp_connection_open_count",
        "increase": 10.0,
        "rate": 10.0 / 60.0,
        "base_unit": "count",
        "rate_unit": "per_second",
    },
    "tcp_closed": {
        "raw": "istio_tcp_connections_closed_total",
        "canonical": "servicegraph.tcp.connection.close.count",
        "vm": "servicegraph_tcp_connection_close_count",
        "increase": 10.0,
        "rate": 10.0 / 60.0,
        "base_unit": "count",
        "rate_unit": "per_second",
    },
    "tcp_sent": {
        "raw": "istio_tcp_sent_bytes_total",
        "canonical": "servicegraph.tcp.bytes.sent",
        "vm": "servicegraph_tcp_bytes_sent",
        "increase": 10240.0,
        "rate": 10240.0 / 60.0,
        "base_unit": "bytes",
        "rate_unit": "bytes_per_second",
    },
    "tcp_received": {
        "raw": "istio_tcp_received_bytes_total",
        "canonical": "servicegraph.tcp.bytes.received",
        "vm": "servicegraph_tcp_bytes_received",
        "increase": 10240.0,
        "rate": 10240.0 / 60.0,
        "base_unit": "bytes",
        "rate_unit": "bytes_per_second",
    },
}


def get_json(url):
    with urllib.request.urlopen(url, timeout=10) as response:
        return json.loads(response.read())


def prometheus_labels(raw_labels):
    return {
        key: bytes(value, "utf-8").decode("unicode_escape")
        for key, value in re.findall(r'(?:^|,)([^=,]+)="((?:\\.|[^"])*)"', raw_labels)
    }


def raw_values_from_lines(lines):
    values = {}
    for key, spec in METRICS.items():
        total = 0.0
        prefix = spec["raw"] + "{"
        for line in lines:
            if not line.startswith(prefix):
                continue
            raw_labels, raw_value = line[len(prefix):].split("}", 1)
            labels = prometheus_labels(raw_labels)
            if labels.get("reporter") != "destination":
                continue
            if labels.get("source_workload") != "mesh-protocol-client":
                continue
            destination_workload = labels.get("destination_workload", "")
            destination_service = labels.get("destination_service_name", "")
            if not (
                service_label_matches(destination_workload, "mesh-protocol-server")
                or service_label_matches(destination_service, "mesh-protocol-server")
            ):
                continue
            total += float(raw_value.strip())
        values[key] = total
    return values


def raw_values():
    with urllib.request.urlopen(RAW_URL, timeout=10) as response:
        lines = response.read().decode().splitlines()
    return raw_values_from_lines(lines)


def vm_values():
    values = {}
    for key, spec in METRICS.items():
        query = f'{spec["vm"]}{{source="mesh.istio"}}'
        payload = get_json(VM_URL + "?" + urllib.parse.urlencode({"query": query, "nocache": "1"}))
        assert payload["status"] == "success", payload
        total = 0.0
        for item in payload["data"]["result"]:
            labels = item["metric"]
            if not service_label_matches(labels.get("edge_client", ""), "mesh-protocol-client"):
                continue
            if not service_label_matches(labels.get("edge_server", ""), "mesh-protocol-server"):
                continue
            total += float(item["value"][1])
        values[key] = total
    return values


def api_payload(metric, aggregation=None):
    params = {"metric_name": metric, "window": "1m", "page_size": "1000"}
    if aggregation:
        params["aggregation"] = aggregation
    return get_json(API_URL + "?" + urllib.parse.urlencode(params)).get("data", {})


def service_label_matches(value, service):
    return value == service or value.startswith(service + ".")


def api_edge_total(metric, aggregation, expected_unit):
    payload = api_payload(metric, aggregation)
    latest = {}
    for sample in payload.get("samples", []):
        labels = sample.get("labels", {})
        if labels.get("source") != "mesh.istio":
            continue
        if not service_label_matches(labels.get("edge_client", ""), "mesh-protocol-client"):
            continue
        if not service_label_matches(labels.get("edge_server", ""), "mesh-protocol-server"):
            continue
        assert sample["unit"] == expected_unit, sample
        series = tuple(sorted(labels.items()))
        if series not in latest or sample["timestamp"] > latest[series]["timestamp"]:
            latest[series] = sample
    if not latest:
        return payload, None
    return payload, sum(float(sample["value"]) for sample in latest.values())


def poll_api_edge_total(metric, aggregation, expected_unit, predicate, timeout=API_POLL_TIMEOUT_SECONDS):
    deadline = time.time() + timeout
    payload = {}
    total = None
    while time.time() < deadline:
        payload, total = api_edge_total(metric, aggregation, expected_unit)
        if total is not None and predicate(total):
            return payload, total
        time.sleep(2)
    assert total is not None, f"no API samples for {metric} aggregation={aggregation} before deadline"
    return payload, total


def capture_baseline():
    # The mesh Prometheus scrape can land near the end of its interval, then
    # still cross the agent/gateway/export boundary. Keep this deadline below
    # the fixture's 180-second warm-up wait so the measured burst can never be
    # mistaken for the baseline.
    deadline = time.time() + 120
    raw = {}
    vm = {}
    while time.time() < deadline:
        raw = raw_values()
        vm = vm_values()
        if all(
            raw.get(key, 0) > 0
            and math.isclose(vm.get(key, 0), raw[key], abs_tol=0.01)
            for key in METRICS
        ):
            break
        time.sleep(3)
    assert all(raw.get(key, 0) > 0 for key in METRICS), raw
    assert all(
        math.isclose(vm.get(key, 0), raw[key], abs_tol=0.01)
        for key in METRICS
    ), {"raw": raw, "vm": vm}
    BASELINE_PATH.write_text(json.dumps({"raw": raw, "vm": vm}, indent=2))
    print("✓ Captured warm gRPC/TCP baselines from the destination sidecar and VictoriaMetrics")


def verify():
    baseline = json.loads(BASELINE_PATH.read_text())
    raw = raw_values()
    for key, spec in METRICS.items():
        raw_delta = raw[key] - baseline["raw"][key]
        assert raw_delta == spec["increase"], f"{key} raw delta {raw_delta}"

    for key, spec in METRICS.items():
        # Istio may expose different protocol counters to the Prometheus scrape
        # at different boundaries. Validate each counter as soon as its measured
        # delta reaches VM so an earlier delta cannot age out of the 1m API
        # window while the verifier waits for a later counter.
        deadline = time.time() + 120
        vm_delta = 0.0
        while time.time() < deadline:
            vm_delta = vm_values()[key] - baseline["vm"][key]
            if math.isclose(vm_delta, spec["increase"], abs_tol=0.01):
                break
            time.sleep(3)

        assert math.isclose(vm_delta, spec["increase"], abs_tol=0.01), f"{key} VM delta {vm_delta}"

    for key, spec in METRICS.items():
        increase_payload, increase = poll_api_edge_total(
            spec["canonical"],
            "increase",
            spec["base_unit"],
            lambda value: math.isclose(value, spec["increase"], abs_tol=0.01),
        )
        assert increase_payload["query"] == {"aggregation": "increase", "window": "1m"}
        assert math.isclose(increase, spec["increase"], abs_tol=0.01), (key, increase)

        default_payload, default_rate = poll_api_edge_total(
            spec["canonical"],
            None,
            spec["rate_unit"],
            lambda value: math.isclose(value, spec["rate"], abs_tol=max(0.02, spec["rate"] * 0.02)),
        )
        assert default_payload["query"] == {"aggregation": "window_rate", "window": "1m"}
        assert math.isclose(default_rate, spec["rate"], abs_tol=max(0.02, spec["rate"] * 0.02)), (key, default_rate)

        _, active_rate = poll_api_edge_total(
            spec["canonical"], "active_rate", spec["rate_unit"], lambda value: value >= 0
        )
        assert active_rate >= 0, (key, active_rate)

    print("✓ Istio raw, VictoriaMetrics, and API gRPC/TCP counters match deterministic traffic")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=["capture-baseline", "verify"])
    args = parser.parse_args()
    if args.command == "capture-baseline":
        capture_baseline()
    else:
        verify()


if __name__ == "__main__":
    main()
