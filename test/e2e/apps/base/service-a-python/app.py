from flask import Flask, jsonify
import os
import requests


app = Flask(__name__)
SERVICE_B_URL = os.getenv(
    "SERVICE_B_URL",
    "http://service-b-node.karthos-demo.svc.cluster.local:8082/compute",
)


@app.route("/process")
def process():
    try:
        response = requests.get(SERVICE_B_URL, timeout=3)
        downstream = response.json()
        status = response.status_code
    except Exception as exc:  # pragma: no cover - test app fallback
        downstream = {"error": str(exc)}
        status = 502

    return (
        jsonify(
            {
                "service": "service-a-python",
                "status": "processed",
                "downstream": downstream,
            }
        ),
        status if status != 200 else 200,
    )


@app.route("/health")
def health():
    return jsonify({"service": "service-a-python", "status": "ok"})


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8081)
