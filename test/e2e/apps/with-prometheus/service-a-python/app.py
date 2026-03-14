from flask import Flask, jsonify, Response
import os
import requests
from prometheus_client import Counter, Histogram, generate_latest


app = Flask(__name__)
SERVICE_B_URL = os.getenv(
    "SERVICE_B_URL",
    "http://service-b-node.oksg-demo.svc.cluster.local:8082/compute",
)

REQUEST_COUNT = Counter(
    "http_requests_total",
    "Total HTTP requests handled by service-a-python",
    ["method", "path", "status"],
)
REQUEST_LATENCY = Histogram(
    "http_request_duration_seconds",
    "Request latency for service-a-python",
    ["method", "path"],
)

@app.route("/process")
def process():
    timer = REQUEST_LATENCY.labels("GET", "/process").time()
    timer.__enter__()
    try:
        try:
            response = requests.get(SERVICE_B_URL, timeout=3)
            downstream = response.json()
            status_code = response.status_code
        except Exception as exc:  # pragma: no cover - test app fallback
            downstream = {"error": str(exc)}
            status_code = 502

        REQUEST_COUNT.labels("GET", "/process", str(status_code)).inc()
        return (
            jsonify(
                {
                    "service": "service-a-python",
                    "status": "processed",
                    "downstream": downstream,
                }
            ),
            status_code if status_code != 200 else 200,
        )
    finally:
        timer.__exit__(None, None, None)


@app.route("/health")
def health():
    REQUEST_COUNT.labels("GET", "/health", "200").inc()
    return jsonify({"service": "service-a-python", "status": "ok"})


@app.route("/metrics")
def metrics():
    return Response(generate_latest(), mimetype="text/plain; version=0.0.4")


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8081)
