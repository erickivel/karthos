import express from "express";

const app = express();
const port = process.env.PORT || 8082;

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

app.listen(port, "0.0.0.0", () => {
  console.log(`service-b-node listening on ${port}`);
});
