import { memo, type CSSProperties, type KeyboardEvent, type MouseEvent } from "react";
import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import { Activity } from "lucide-react";
import { useAppStore } from "@/stores/useAppStore";
import type { GraphNode, EntityKind } from "@/types/graph";
import type { NodeMetricValues } from "@/hooks/useNodeMetrics";
import { topologyEntityTestId } from "../testIds";
import { getNodeMetricSpecs, type MetricSelector } from "../nodeMetrics";
import { NodeMetric } from "./NodeMetric";
import styles from "./EntityNode.module.css";
import { namespaceColor } from "../namespaceColor";
import type { ProjectionMode } from "../projection";

type GraphNodeData = GraphNode & { metrics?: NodeMetricValues; mode?: ProjectionMode } & Record<string, unknown>;

const KIND_COLOR_VAR: Record<EntityKind, string> = {
  cluster: "var(--kind-cluster)",
  namespace: "var(--kind-namespace)",
  node: "var(--kind-node)",
  service: "var(--kind-service)",
  workload: "var(--kind-workload)",
  pod: "var(--kind-pod)",
  container: "var(--kind-container)",
  ingress: "var(--kind-ingress)",
  endpoint: "var(--kind-endpoint)",
  configmap: "var(--kind-configmap)",
  secret: "var(--kind-secret)",
  pvc: "var(--kind-pvc)",
};

function getHeaderColor(
  kind: EntityKind,
  status: string,
  mode: ProjectionMode | undefined,
  namespace: string | undefined,
): string {
  if (status === "error") return "var(--color-alert)";
  if (kind === "pod" && mode === "infrastructure" && namespace) {
    return namespaceColor(namespace);
  }
  return KIND_COLOR_VAR[kind] ?? "var(--surface-active)";
}

const SMALL_KINDS: EntityKind[] = ["configmap", "secret", "pvc"];

function getDisplayName(node: GraphNode): string {
  return `${node.kind}_${node.name}`
    .replace(/[^a-zA-Z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .toUpperCase();
}

function getPrimarySignal(node: GraphNode): { label: string; value: string; tone: string } {
  const labels = node.labels ?? {};
  const phase = labels["k8s.pod.phase"];
  if (node.status === "error" || node.status === "critical") {
    return { label: "Exception detected", value: "CRITICAL", tone: "alert" };
  }
  if (phase === "2") return { label: "Status", value: "RUNNING", tone: "signal" };
  if (node.kind === "service") {
    return { label: "Endpoint", value: node.name, tone: "signal" };
  }
  if (node.namespace) {
    return { label: "Namespace", value: node.namespace, tone: "info" };
  }
  return { label: "Status", value: node.status.toUpperCase(), tone: "muted" };
}

function metricValue(values: NodeMetricValues, selector: MetricSelector | undefined): number | undefined {
  if (!selector) return undefined;
  if (typeof selector === "string") {
    return values[selector];
  }
  for (const metric of selector) {
    if (values[metric] !== undefined) return values[metric];
  }
  return undefined;
}

function EntityNodeComponent({
  data,
  sourcePosition,
  targetPosition,
}: NodeProps<Node<GraphNodeData>>) {
  const node = data;
  const sourceP = sourcePosition ?? Position.Bottom;
  const targetP = targetPosition ?? Position.Top;
  const setSelectedEntityId = useAppStore((s) => s.setSelectedEntityId);
  const isSmall = SMALL_KINDS.includes(node.kind);
  const metrics: NodeMetricValues = node.metrics ?? {};
  const hasTelemetry =
    node.status !== "unknown" ||
    Object.keys(node.labels ?? {}).length > 0 ||
    Object.keys(metrics).length > 0;
  const specs = getNodeMetricSpecs(node.kind);
  const hasMetricData = specs.some((spec) => {
    switch (spec.kind) {
      case 'utilization':
        return metricValue(metrics, spec.metric) !== undefined;
      case 'fraction':
        return (
          metricValue(metrics, spec.numeratorMetric) !== undefined &&
          (metricValue(metrics, spec.denominatorMetric) !== undefined ||
            (spec.denominatorMetadataKey !== undefined &&
              Number.isFinite(Number(node.labels?.[spec.denominatorMetadataKey]))))
        );
      case 'rate':
        return metricValue(metrics, spec.metric) !== undefined;
      case 'gauge':
        return metricValue(metrics, spec.metric) !== undefined;
    }
  });
  const primarySignal = getPrimarySignal(node);
  const selectNode = () => setSelectedEntityId(node.id);
  const handleClick = (event: MouseEvent<HTMLDivElement>) => {
    event.stopPropagation();
    selectNode();
  };
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      event.stopPropagation();
      selectNode();
    }
  };

  return (
    <div
      className={styles.card}
      data-small={isSmall}
      data-no-telemetry={!hasTelemetry}
      data-testid={topologyEntityTestId(node.kind, node.name)}
      data-entity-kind={node.kind}
      data-entity-name={node.name}
      data-entity-namespace={node.namespace}
      style={{ "--node-accent": getHeaderColor(node.kind, node.status, node.mode, node.namespace) } as CSSProperties}
      data-namespace-color={
        node.kind === 'pod' && node.mode === 'infrastructure' && node.namespace
          ? namespaceColor(node.namespace)
          : undefined
      }
      onClick={handleClick}
      onKeyDown={handleKeyDown}
      role="button"
      tabIndex={0}
    >
      <Handle type="target" position={targetP} className={styles.handle} />

      <div
        className={styles.header}
        data-tone={primarySignal.tone}
      >
        <span className={styles.name}>{getDisplayName(node)}</span>
        <Activity size={13} strokeWidth={2} className={styles.icon} />
      </div>

      <div className={styles.body}>
        <div className={styles.signalBlock} data-tone={primarySignal.tone}>
          <span className={styles.propLabel}>{primarySignal.label}</span>
          <span className={styles.propValue}>{primarySignal.value}</span>
        </div>

        {hasMetricData && (
          <div className={styles.metrics} data-topology-dynamic="metrics">
            {specs.map((spec, idx) => (
              <NodeMetric
                key={`${spec.kind}-${idx}`}
                spec={spec}
                values={metrics}
                metadata={node.labels}
              />
            ))}
          </div>
        )}

        {!hasTelemetry && (
          <div className={styles.telemetryHint}>Awaiting telemetry data</div>
        )}
      </div>

      <Handle
        type="source"
        position={sourceP}
        className={styles.handle}
      />
    </div>
  );
}

export const EntityNode = memo(EntityNodeComponent);
