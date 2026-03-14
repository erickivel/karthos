import express from "express";
import client from "prom-client";

const app = express();
const port = process.env.PORT || 8082;

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

  const end = requestDuration.startTimer({ method: req.method, path: req.path });
  res.on("finish", () => {
    requestCounter.inc({ method: req.method, path: req.path, status: String(res.statusCode) });
    end();
  });
  next();
});

app.get("/compute", (_req, res) => {
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
