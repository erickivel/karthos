import os
import time
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit

import requests
from flask import Flask, Response, jsonify, request
from opentelemetry import context as otel_context
from opentelemetry import propagate, trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.trace import SpanKind, Status, StatusCode
from prometheus_client import Counter, Histogram, generate_latest


def init_tracer():
    endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
    service_name = os.getenv("OTEL_SERVICE_NAME", "service-a-python")

    provider = TracerProvider(resource=Resource.create({"service.name": service_name}))
    provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter(endpoint=f"{endpoint}/v1/traces")))
    trace.set_tracer_provider(provider)
    return provider


app = Flask(__name__)
SERVICE_B_URL = os.getenv(
    "SERVICE_B_URL",
    "http://service-b-node.karthos-demo.svc.cluster.local:8082/compute",
)
TRACER_PROVIDER = init_tracer()
TRACER = trace.get_tracer("service-a-python")

REQUEST_COUNT = Counter(
    "http_requests_total",
    "Total HTTP requests handled by service-a-python",
    ["method", "http_route", "status"],
)
REQUEST_LATENCY = Histogram(
    "http_request_duration_seconds",
    "Request latency for service-a-python",
    ["method", "http_route"],
)


def route_template():
    if request.url_rule is not None:
        return request.url_rule.rule
    return "/{path}"


def with_force_error(raw_url):
    parts = urlsplit(raw_url)
    query = dict(parse_qsl(parts.query, keep_blank_values=True))
    query["force_error"] = "true"
    return urlunsplit((parts.scheme, parts.netloc, parts.path, urlencode(query), parts.fragment))


@app.before_request
def start_request_span():
    if request.path == "/metrics":
        return

    parent_context = propagate.extract(request.headers)
    span = TRACER.start_span(f"{request.method} {route_template()}", context=parent_context, kind=SpanKind.SERVER)
    token = otel_context.attach(trace.set_span_in_context(span, parent_context))
    request.environ["otel.span"] = span
    request.environ["otel.token"] = token
    request.environ["otel.start_time"] = time.perf_counter()


@app.after_request
def finish_request_span(response):
    span = request.environ.get("otel.span")
    token = request.environ.get("otel.token")
    started = request.environ.get("otel.start_time")
    route = route_template()

    if started is not None and request.path != "/metrics":
        REQUEST_LATENCY.labels(request.method, route).observe(time.perf_counter() - started)
        REQUEST_COUNT.labels(request.method, route, str(response.status_code)).inc()

    if span is not None:
        span.set_attribute("http.method", request.method)
        span.set_attribute("http.route", route)
        span.set_attribute("http.status_code", response.status_code)
        if response.status_code >= 500:
            span.set_status(Status(StatusCode.ERROR, str(response.status_code)))
        span.end()
    if token is not None:
        otel_context.detach(token)
    return response


@app.route("/process")
def process():
    with TRACER.start_as_current_span("call-service-b-node", kind=SpanKind.CLIENT) as span:
        span.set_attribute("peer.service", "service-b-node")
        span.set_attribute("server_namespace", os.getenv("DESTINATION_NAMESPACE", "karthos-demo"))
        headers = {}
        propagate.inject(headers)
        downstream_url = with_force_error(SERVICE_B_URL) if request.args.get("force_error") == "true" else SERVICE_B_URL

        try:
            response = requests.get(downstream_url, timeout=3, headers=headers)
            downstream = response.json()
            status_code = response.status_code
            if status_code >= 500:
                span.set_status(Status(StatusCode.ERROR, str(status_code)))
        except Exception as exc:  # pragma: no cover - test app fallback
            span.record_exception(exc)
            span.set_status(Status(StatusCode.ERROR, str(exc)))
            downstream = {"error": str(exc)}
            status_code = 502

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


@app.route("/health")
def health():
    return jsonify({"service": "service-a-python", "status": "ok"})


@app.route("/metrics")
def metrics():
    return Response(generate_latest(), mimetype="text/plain; version=0.0.4")


if __name__ == "__main__":
    try:
        app.run(host="0.0.0.0", port=8081)
    finally:
        TRACER_PROVIDER.shutdown()
