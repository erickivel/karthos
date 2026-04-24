import { useNavigate } from "react-router-dom";
import { useNodeDetail } from "@/hooks/useNodeDetail";
import { useAppStore } from "@/stores/useAppStore";
import { DataChip } from "@/components/shared/DataChip";
import type { GraphNode } from "@/types/graph";
import { formatBytes } from "../../utils/formatters";
import { EdgeMetricButton } from "./EdgeMetricButton";
import styles from "./NodeInspector.module.css";

interface NodeMetricSummaryItem {
  label: string;
  value: string;
  tone?: "signal" | "info" | "alert" | "muted";
}

function parseLabelNumber(
  labels: Record<string, string>,
  key: string,
): number | null {
  const raw = labels[key];
  if (raw === undefined) return null;
  const v = Number(raw);
  return Number.isFinite(v) ? v : null;
}

/**
 * buildMetricSummary reads synthesised node labels already attached by the backend
 * and `useGraphLayout`: `k8s.pod.cpu.usage`, `k8s.pod.memory.working_set`,
 * `k8s.pod.phase`, `k8s.node.condition.ready`, `k8s.node.condition.memory_pressure`,
 * and `k8s.node.condition.disk_pressure`. The canonical OTel-to-label mapping in
 * `pkg/core/metrics/otel_mappings.go` is the authoritative source for these keys.
 */
function buildMetricSummary(node: GraphNode): NodeMetricSummaryItem[] {
  const labels = node.labels ?? {};
  const items: NodeMetricSummaryItem[] = [];

  const cpu = parseLabelNumber(labels, "k8s.pod.cpu.usage");
  if (cpu !== null) {
    items.push({ label: "CPU", value: `${Math.round(cpu)} m`, tone: "info" });
  }
  const mem = parseLabelNumber(labels, "k8s.pod.memory.working_set");
  if (mem !== null) {
    items.push({ label: "Memory", value: formatBytes(mem), tone: "info" });
  }

  const phase = parseLabelNumber(labels, "k8s.pod.phase");
  if (phase !== null) {
    // OTel k8sclusterreceiver encoding: 1=Pending, 2=Running, 3=Succeeded, 4=Failed, 5=Unknown.
    const name =
      phase === 1
        ? "Pending"
        : phase === 2
          ? "Running"
          : phase === 3
            ? "Succeeded"
            : phase === 4
              ? "Failed"
              : "Unknown";
    const tone: NodeMetricSummaryItem["tone"] =
      name === "Running"
        ? "signal"
        : name === "Pending"
          ? "info"
          : name === "Failed"
            ? "alert"
            : "muted";
    items.push({ label: "Phase", value: name, tone });
  }

  const ready = parseLabelNumber(labels, "k8s.node.condition.ready");
  if (ready !== null) {
    items.push({
      label: "Ready",
      value: ready >= 1 ? "Ready" : "Not Ready",
      tone: ready >= 1 ? "signal" : "alert",
    });
  }
  const memPressure = parseLabelNumber(
    labels,
    "k8s.node.condition.memory_pressure",
  );
  if (memPressure !== null) {
    items.push({
      label: "Memory Pressure",
      value: memPressure >= 1 ? "Detected" : "Clear",
      tone: memPressure >= 1 ? "alert" : "signal",
    });
  }
  const diskPressure = parseLabelNumber(labels, "k8s.node.condition.disk_pressure");
  if (diskPressure !== null) {
    items.push({
      label: "Disk Pressure",
      value: diskPressure >= 1 ? "Detected" : "Clear",
      tone: diskPressure >= 1 ? "alert" : "signal",
    });
  }

  return items;
}

function getDetailItems(node: GraphNode) {
  const labels = node.labels ?? {};

  if (node.kind === "workload") {
    const workloadType = labels["k8s.deployment.name"]
      ? "Deployment"
      : labels["k8s.statefulset.name"]
        ? "StatefulSet"
        : labels["k8s.daemonset.name"]
          ? "DaemonSet"
          : labels["k8s.replicaset.name"]
            ? "ReplicaSet"
            : "Workload";

    return [
      { label: "Type", value: workloadType },
      { label: "Controller", value: node.name },
      ...(node.namespace
        ? [{ label: "Namespace", value: node.namespace }]
        : []),
    ];
  }

  if (node.kind === "namespace") {
    return [
      { label: "Namespace", value: node.name },
      ...(node.cluster ? [{ label: "Cluster", value: node.cluster }] : []),
    ];
  }

  if (node.kind === "node") {
    return [
      { label: "Node", value: node.name },
      ...(node.cluster ? [{ label: "Cluster", value: node.cluster }] : []),
    ];
  }

  return [
    ...(node.cluster ? [{ label: "Cluster", value: node.cluster }] : []),
    ...(node.namespace ? [{ label: "Namespace", value: node.namespace }] : []),
  ];
}

export function NodeInspector() {
  const navigate = useNavigate();
  const selectedEntityId = useAppStore((s) => s.selectedEntityId);
  const setSelectedEntityId = useAppStore((s) => s.setSelectedEntityId);
  const { data } = useNodeDetail(selectedEntityId);

  if (!selectedEntityId || !data) return null;

  const metricSummary = buildMetricSummary(data.node);

  const { node, edges } = data;
  const labels = node.labels ?? {};
  const detailItems = getDetailItems(node);
  const openEdgeMetrics = (sourceId: string, targetId: string) => {
    navigate(
      `/metrics?mode=edge&edgeSource=${encodeURIComponent(sourceId)}&edgeTarget=${encodeURIComponent(targetId)}`,
    );
  };

  return (
    <div className={styles.panel} data-testid="node-inspector">
      <div className={styles.header}>
        <div className={styles.titleRow}>
          <span className={styles.dot} data-status={node.status} />
          <h2 className={styles.name} data-testid="node-inspector-title">
            {node.name}
          </h2>
        </div>
        <button
          className={styles.closeBtn}
          onClick={() => setSelectedEntityId(null)}
          aria-label="Close inspector"
        >
          x
        </button>
      </div>

      <div className={styles.chips}>
        <DataChip label={node.kind} variant="signal" />
        {node.namespace && <DataChip label={node.namespace} />}
      </div>

      {(detailItems.length > 0 || metricSummary.length > 0) && (
        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>Details</h3>

          {detailItems.length > 0 && (
            <div className={styles.propList}>
              {detailItems.map((item) => (
                <div key={item.label} className={styles.prop}>
                  <span className={styles.propKey}>{item.label}</span>
                  <span className={styles.propVal}>{item.value}</span>
                </div>
              ))}
            </div>
          )}

          {metricSummary.length > 0 && (
            <div className={styles.summaryGrid}>
              {metricSummary.map((item) => (
                <div
                  key={item.label}
                  className={styles.summaryCard}
                  data-tone={item.tone ?? "muted"}
                >
                  <span className={styles.summaryLabel}>{item.label}</span>
                  <span className={styles.summaryValue}>{item.value}</span>
                </div>
              ))}
            </div>
          )}
        </section>
      )}

      <section className={styles.section}>
        <h3 className={styles.sectionTitle}>Properties</h3>
        <div className={styles.propList}>
          {Object.entries(labels).map(([key, value]) => (
            <div key={key} className={styles.prop}>
              <span className={styles.propKey}>{key}</span>
              <span className={styles.propVal}>{value}</span>
            </div>
          ))}
          {Object.keys(labels).length === 0 && (
            <div className={styles.prop}>
              <span className={styles.propVal}>No properties available</span>
            </div>
          )}
        </div>
      </section>

      {edges.inbound.length > 0 && (
        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>
            Inbound ({edges.inbound.length})
          </h3>
          <div className={styles.edgeList}>
            {edges.inbound.map((e) => (
              <div key={e.id} className={styles.edgeItem}>
                <button
                  className={styles.edgePrimary}
                  onClick={() =>
                    e.related_node && setSelectedEntityId(e.related_node.id)
                  }
                >
                  <span className={styles.edgeRelation}>{e.relation}</span>
                  <span className={styles.edgeEntity}>
                    {e.related_node?.name || String(e.source)}
                  </span>
                </button>
                <EdgeMetricButton edge={e} onNavigate={openEdgeMetrics} />
              </div>
            ))}
          </div>
        </section>
      )}

      {edges.outbound.length > 0 && (
        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>
            Outbound ({edges.outbound.length})
          </h3>
          <div className={styles.edgeList}>
            {edges.outbound.map((e) => (
              <div key={e.id} className={styles.edgeItem}>
                <button
                  className={styles.edgePrimary}
                  onClick={() =>
                    e.related_node && setSelectedEntityId(e.related_node.id)
                  }
                >
                  <span className={styles.edgeRelation}>{e.relation}</span>
                  <span className={styles.edgeEntity}>
                    {e.related_node?.name || String(e.target)}
                  </span>
                </button>
                <EdgeMetricButton edge={e} onNavigate={openEdgeMetrics} />
              </div>
            ))}
          </div>
        </section>
      )}

      <button
        className={styles.metricsLink}
        onClick={() =>
          navigate(`/metrics?entity=${encodeURIComponent(selectedEntityId)}`)
        }
      >
        View Full Metrics &rarr;
      </button>
    </div>
  );
}
