import { useMemo, type ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import {
  ArrowDown,
  ArrowUp,
  BarChart3,
  Network,
  Tags,
  X,
} from "lucide-react";
import { useNodeDetail } from "@/hooks/useNodeDetail";
import { useEdgeMetrics } from "@/hooks/useEdgeMetrics";
import { useGraphSnapshot } from "@/hooks/useGraphSnapshot";
import { useAppStore } from "@/stores/useAppStore";
import { DataChip } from "@/components/shared/DataChip";
import type { GraphNode } from "@/types/graph";
import { formatBytes } from "../../utils/formatters";
import { EdgeMetricButton } from "./EdgeMetricButton";
import { buildEdgeMetricKey, type EdgeMetricSnapshot } from "./edgeMetrics";
import { edgeMetricsUrl, entityMetricsUrl } from "@/lib/links";
import styles from "./NodeInspector.module.css";

interface NodeMetricSummaryItem {
  label: string;
  value: string;
  tone?: "signal" | "info" | "alert" | "muted";
}

interface InfoItem {
  label: string;
  value: string;
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

// Reads only metric labels already attached by the backend or graph layout.
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

function MeshEdgeChips({ snapshot }: { snapshot: EdgeMetricSnapshot | undefined }) {
  if (!snapshot) return null;
  const hasMesh = snapshot.mtls !== undefined || snapshot.protocol;
  if (!hasMesh) return null;
  const tooltipParts: string[] = [];
  if (snapshot.peerSource) tooltipParts.push(`source: ${snapshot.peerSource}`);
  if (snapshot.peerDestination) tooltipParts.push(`destination: ${snapshot.peerDestination}`);
  if (snapshot.source) tooltipParts.push(`from ${snapshot.source}`);
  const title = tooltipParts.join("\n") || undefined;
  return (
    <span className={styles.edgeMeshChips} title={title}>
      {snapshot.mtls && <DataChip label="mTLS" variant="signal" />}
      {snapshot.protocol && <DataChip label={snapshot.protocol} variant="info" />}
    </span>
  );
}

function InspectorDataCard({
  children,
  description,
  icon,
  tone = "neutral",
  title,
}: {
  children: ReactNode;
  description: string;
  icon: ReactNode;
  tone?: "neutral" | "meta" | "props" | "inbound" | "outbound";
  title: string;
}) {
  return (
    <section className={styles.dataCard}>
      <div className={styles.dataCardHeader}>
        <span className={styles.dataCardIcon} data-tone={tone}>
          {icon}
        </span>
        <div className={styles.dataCardTitleBlock}>
          <h3 className={styles.dataCardTitle}>{title}</h3>
          <span className={styles.dataCardDescription}>{description}</span>
        </div>
      </div>
      <div className={styles.dataCardBody}>{children}</div>
    </section>
  );
}

function DataRow({ item }: { item: InfoItem }) {
  return (
    <div className={styles.dataRow}>
      <span className={styles.dataKey}>{item.label}</span>
      <strong className={styles.dataValue}>{item.value}</strong>
    </div>
  );
}

function SkeletonEdgeRows({ count = 3 }: { count?: number }) {
  return (
    <div className={styles.edgeList} aria-busy="true">
      {Array.from({ length: count }).map((_, i) => (
        <div key={i} className={styles.skeletonRow}>
          <span className={styles.skeletonDot} />
          <span className={styles.skeletonBar} />
          <span className={`${styles.skeletonBar} ${styles.skeletonBarShort}`} />
        </div>
      ))}
    </div>
  );
}

function EdgeCardError({ onRetry }: { onRetry: () => void }) {
  return (
    <div className={styles.errorState}>
      <span className={styles.errorMessage}>Couldn't load edge details</span>
      <button type="button" className={styles.retryBtn} onClick={onRetry}>
        Retry
      </button>
    </div>
  );
}

function getDetailItems(node: GraphNode) {
  const labels = node.labels ?? {};
  const dash = "—";

  const base: InfoItem[] = [
    { label: "Kind", value: node.kind ? String(node.kind) : dash },
    { label: "Cluster", value: node.cluster || dash },
    { label: "Namespace", value: node.namespace || dash },
  ];

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
      ...base,
      { label: "Type", value: workloadType },
      { label: "Controller", value: node.name },
    ];
  }

  return base;
}

function getSpecItems(node: GraphNode) {
  const labels = node.labels ?? {};
  return [
    ...(labels["architecture"]
      ? [{ label: "Architecture", value: labels["architecture"] }]
      : []),
    ...(labels["kernel"] ? [{ label: "Kernel", value: labels["kernel"] }] : []),
    ...(labels["instance"] ? [{ label: "Instance", value: labels["instance"] }] : []),
    ...(node.namespace ? [{ label: "Namespace", value: node.namespace }] : []),
  ];
}

export function NodeInspector() {
  const navigate = useNavigate();
  const selectedEntityId = useAppStore((s) => s.selectedEntityId);
  const setSelectedEntityId = useAppStore((s) => s.setSelectedEntityId);
  const { data, isPending, isError, refetch } = useNodeDetail(selectedEntityId);
  const edgeMetrics = useEdgeMetrics();
  const { data: graphSnapshot } = useGraphSnapshot();

  const snapshotNode = useMemo(() => {
    if (!selectedEntityId || !graphSnapshot) return undefined;
    return graphSnapshot.graph.nodes.find((n) => n.id === selectedEntityId);
  }, [selectedEntityId, graphSnapshot]);

  if (!selectedEntityId) return null;

  const node = data?.node ?? snapshotNode;
  if (!node) return null;

  const edges = data?.edges;
  const detailLoading = isPending && !data;
  const detailFailed = isError && !data;
  const metricSummary = buildMetricSummary(node);
  const labels = node.labels ?? {};
  const detailItems = getDetailItems(node);
  const specItems = getSpecItems(node);
  const openEdgeMetrics = (sourceId: string, targetId: string) => {
    navigate(edgeMetricsUrl(sourceId, targetId));
  };
  const openEntityMetrics = () => {
    navigate(entityMetricsUrl(node.id));
  };

  return (
    <div className={styles.panel} data-testid="node-inspector">
      <div className={styles.header}>
        <div className={styles.titleBlock}>
          <span className={styles.eyebrow}>Node Inspector</span>
          <h2
            className={styles.name}
            data-kind={node.kind}
            data-testid="node-inspector-title"
          >
            {node.name}
          </h2>
          <button
            className={styles.detailMetricsLink}
            onClick={openEntityMetrics}
            aria-label={`Open detailed metrics for ${node.name}`}
          >
            <BarChart3 size={13} strokeWidth={2} />
            <span>Detailed metrics</span>
          </button>
          {detailLoading && (
            <span className={styles.loadingSubtitle}>Loading details…</span>
          )}
        </div>
        <button
          className={styles.closeBtn}
          onClick={() => setSelectedEntityId(null)}
          aria-label="Close inspector"
        >
          <X size={16} strokeWidth={2} />
        </button>
      </div>

      {specItems.length > 0 && (
        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>Technical Specs</h3>

          <div className={styles.specList}>
            {specItems.map((item) => (
              <div key={item.label} className={styles.specRow}>
                <span className={styles.propKey}>{item.label}</span>
                <span className={styles.propVal}>{item.value}</span>
              </div>
            ))}
          </div>
        </section>
      )}

      <section className={styles.section}>
        <h3 className={styles.sectionTitle}>Live Telemetry</h3>
        <div className={styles.telemetryPanel}>
          {metricSummary.length > 0 ? (
            <div className={styles.telemetryList}>
              {metricSummary.map((item) => (
                <div
                  key={item.label}
                  className={styles.telemetryRow}
                  data-tone={item.tone ?? "muted"}
                >
                  <span>{item.label}</span>
                  <strong>{item.value}</strong>
                </div>
              ))}
            </div>
          ) : (
            <span className={styles.emptyState}>No live telemetry for this entity</span>
          )}
        </div>
      </section>

      <div className={styles.dataCardStack}>
        <InspectorDataCard
          description="where this entity belongs"
          icon={<Network size={15} strokeWidth={2} />}
          title="Graph Metadata"
          tone="meta"
        >
          {detailItems.length > 0 ? (
            detailItems.map((item) => <DataRow key={item.label} item={item} />)
          ) : (
            <span className={styles.emptyState}>No graph metadata available</span>
          )}
        </InspectorDataCard>

        <InspectorDataCard
          description="labels and raw facts"
          icon={<Tags size={15} strokeWidth={2} />}
          title="Properties"
          tone="props"
        >
          {Object.entries(labels).length > 0 ? (
            Object.entries(labels).map(([key, value]) => (
              <DataRow key={key} item={{ label: key, value }} />
            ))
          ) : (
            <span className={styles.emptyState}>No properties available</span>
          )}
        </InspectorDataCard>

        <InspectorDataCard
          description="top to bottom into this node"
          icon={<ArrowDown size={15} strokeWidth={2} />}
          title="Inbound"
          tone="inbound"
        >
          {detailLoading ? (
            <SkeletonEdgeRows />
          ) : detailFailed ? (
            <EdgeCardError onRetry={() => refetch()} />
          ) : edges && edges.inbound.length > 0 ? (
            <div className={styles.edgeList}>
              {edges.inbound.map((e) => {
                const snapshot =
                  e.relation === "calls" && e.related_node?.name
                    ? edgeMetrics.get(buildEdgeMetricKey(e.related_node.name, node.name))
                    : undefined;
                const relatedName = e.related_node?.name || String(e.source);
                return (
                  <div key={e.id} className={styles.edgeItem}>
                    <button
                      className={styles.edgePrimary}
                      onClick={() =>
                        e.related_node && setSelectedEntityId(e.related_node.id)
                      }
                      aria-label={`Select inbound ${relatedName}`}
                    >
                      <span className={styles.directionIcon} data-direction="inbound">
                        <ArrowDown size={14} strokeWidth={2} />
                      </span>
                      <span className={styles.edgeEntity}>{relatedName}</span>
                      <span className={styles.edgeRelation}>{e.relation}</span>
                      <MeshEdgeChips snapshot={snapshot} />
                    </button>
                    <EdgeMetricButton edge={e} onNavigate={openEdgeMetrics} />
                  </div>
                );
              })}
            </div>
          ) : (
            <span className={styles.emptyState}>No inbound relations</span>
          )}
        </InspectorDataCard>

        <InspectorDataCard
          description="bottom to top away from this node"
          icon={<ArrowUp size={15} strokeWidth={2} />}
          title="Outbound"
          tone="outbound"
        >
          {detailLoading ? (
            <SkeletonEdgeRows />
          ) : detailFailed ? (
            <EdgeCardError onRetry={() => refetch()} />
          ) : edges && edges.outbound.length > 0 ? (
            <div className={styles.edgeList}>
              {edges.outbound.map((e) => {
                const snapshot =
                  e.relation === "calls" && e.related_node?.name
                    ? edgeMetrics.get(buildEdgeMetricKey(node.name, e.related_node.name))
                    : undefined;
                const relatedName = e.related_node?.name || String(e.target);
                return (
                  <div key={e.id} className={styles.edgeItem}>
                    <button
                      className={styles.edgePrimary}
                      onClick={() =>
                        e.related_node && setSelectedEntityId(e.related_node.id)
                      }
                      aria-label={`Select outbound ${relatedName}`}
                    >
                      <span className={styles.directionIcon} data-direction="outbound">
                        <ArrowUp size={14} strokeWidth={2} />
                      </span>
                      <span className={styles.edgeEntity}>{relatedName}</span>
                      <span className={styles.edgeRelation}>{e.relation}</span>
                      <MeshEdgeChips snapshot={snapshot} />
                    </button>
                    <EdgeMetricButton edge={e} onNavigate={openEdgeMetrics} />
                  </div>
                );
              })}
            </div>
          ) : (
            <span className={styles.emptyState}>No outbound relations</span>
          )}
        </InspectorDataCard>
      </div>
    </div>
  );
}
