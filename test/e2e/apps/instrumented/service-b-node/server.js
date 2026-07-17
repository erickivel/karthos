import express from "express";
import { context, propagation, trace, SpanKind, SpanStatusCode } from "@opentelemetry/api";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http";
import resources from "@opentelemetry/resources";
import sdkTraceBase from "@opentelemetry/sdk-trace-base";
import sdkTraceNode from "@opentelemetry/sdk-trace-node";
import client from "prom-client";

const { Resource } = resources;
const { BatchSpanProcessor } = sdkTraceBase;
const { NodeTracerProvider } = sdkTraceNode;

const app = express();
const port = process.env.PORT || 8082;
const otlpEndpoint = process.env.OTEL_EXPORTER_OTLP_ENDPOINT || "http://localhost:4318";
const serviceName = process.env.OTEL_SERVICE_NAME || "service-b-node";

const tracerProvider = new NodeTracerProvider({
  resource: new Resource({
    "service.name": serviceName,
  }),
});
tracerProvider.addSpanProcessor(
  new BatchSpanProcessor(new OTLPTraceExporter({ url: `${otlpEndpoint}/v1/traces` })),
);
tracerProvider.register();

const tracer = trace.getTracer("service-b-node");

const register = new client.Registry();
client.collectDefaultMetrics({ register });

const requestCounter = new client.Counter({
  name: "http_requests_total",
  help: "Total HTTP requests handled by service-b-node",
  labelNames: ["method", "path", "status"],
  registers: [register],
});

const requestDuration = new client.Histogram({
  name: "http_request_duration_seconds",
  help: "Request duration in seconds for service-b-node",
  labelNames: ["method", "path"],
  registers: [register],
});

app.use((req, res, next) => {
  if (req.path === "/metrics") {
    next();
    return;
  }

  const parentContext = propagation.extract(context.active(), req.headers);
  const span = tracer.startSpan(
    `${req.method} ${req.path}`,
    {
      kind: SpanKind.SERVER,
      attributes: {
        "http.method": req.method,
        "http.route": req.path,
      },
    },
    parentContext,
  );
  const spanContext = trace.setSpan(parentContext, span);
  const end = requestDuration.startTimer({ method: req.method, path: req.path });

  res.on("finish", () => {
    requestCounter.inc({ method: req.method, path: req.path, status: String(res.statusCode) });
    span.setAttribute("http.status_code", res.statusCode);
    if (res.statusCode >= 500) {
      span.setStatus({ code: SpanStatusCode.ERROR, message: String(res.statusCode) });
    }
    span.end();
    end();
  });

  context.with(spanContext, () => next());
});

app.get("/compute", (req, res) => {
  if (req.query.force_error === "true") {
    res.status(500).json({
      service: "service-b-node",
      status: "intentional-error",
      error: "forced servicegraph error fixture",
    });
    return;
  }

  res.json({
    service: "service-b-node",
    result: 42,
  });
});

app.get("/health", (_req, res) => {
  res.json({
    service: "service-b-node",
    status: "ok",
  });
});

app.get("/metrics", async (_req, res) => {
  res.set("Content-Type", register.contentType);
  res.end(await register.metrics());
});

app.listen(port, "0.0.0.0", () => {
  console.log(`service-b-node listening on ${port}`);
});

process.on("SIGTERM", async () => {
  await tracerProvider.shutdown();
  process.exit(0);
});
