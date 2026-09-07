import { memo, type CSSProperties, type KeyboardEvent, type MouseEvent } from "react";
import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import { Activity, ArrowUpRight } from "lucide-react";
import { useAppStore } from "@/stores/useAppStore";
import type { GraphNode, EntityKind } from "@/types/graph";
import type { NodeMetricValues } from "@/hooks/useNodeMetrics";
import { topologyEntityTestId } from "../testIds";
import { getNodeMetricSpecs, type MetricSelector } from "../nodeMetrics";
import { NodeMetric } from "./NodeMetric";
import styles from "./EntityNode.module.css";
import { namespaceColor } from "../namespaceColor";
import type { ProjectionMode } from "../projection";
import { presentEntityNode } from "./nodePresentation";
import type { DependencyLinkCounts } from "../ownershipFocus";

type GraphNodeData = GraphNode & {
  metrics?: NodeMetricValues;
  mode?: ProjectionMode;
  dependencyLinks?: DependencyLinkCounts;
} & Record<string, unknown>;

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
  hpa: "var(--kind-workload)",
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

function metricValue(values: NodeMetricValues, selector: MetricSelector | undefined) {
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
  const openRelationshipCard = useAppStore((s) => s.openRelationshipCard);
  const isSmall = SMALL_KINDS.includes(node.kind);
  const metrics: NodeMetricValues = node.metrics ?? {};
  const hasTelemetry =
    node.status !== "unknown" ||
    Object.keys(node.labels ?? {}).length > 0 ||
    Object.keys(metrics).length > 0;
  const specs = getNodeMetricSpecs(node.kind);
  const hasMetricData = specs.some((spec) => metricValue(metrics, spec.metric) !== undefined);
  const presentation = presentEntityNode(node, node.mode);
  const primarySignal = presentation.signal;
  const dependencySummary = formatDependencySummary(node.dependencyLinks);
  const hasBodyContent = Boolean(primarySignal) || hasMetricData || Boolean(dependencySummary) || !hasTelemetry;
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
        data-tone={primarySignal?.tone}
      >
        <span className={styles.identity}>
          <span className={styles.kind}>{presentation.kind}</span>
          <span className={styles.name}>{presentation.name}</span>
        </span>
        <Activity size={13} strokeWidth={2} className={styles.icon} />
      </div>

      {hasBodyContent && (
        <div className={styles.body}>
          {primarySignal && (
            <div className={styles.signalBlock} data-tone={primarySignal.tone}>
              <span className={styles.propLabel}>{primarySignal.label}</span>
              <span className={styles.propValue}>{primarySignal.value}</span>
            </div>
          )}

          {hasMetricData && (
            <div className={styles.metrics} data-topology-dynamic="metrics">
              {specs.map((spec, idx) => (
                <NodeMetric
                key={`${spec.label}-${idx}`}
                spec={spec}
                values={metrics}
                entityId={node.id}
                />
              ))}
            </div>
          )}

          {dependencySummary && (
            <button
              type="button"
              className={styles.dependencySummary}
              onClick={(event) => {
                event.stopPropagation();
                openRelationshipCard(node.id);
              }}
              aria-label={`Open relationships for ${node.name}`}
            >
              <span>{dependencySummary}</span>
              <span className={styles.dependencyAction}>Open <ArrowUpRight size={11} strokeWidth={2.5} /></span>
            </button>
          )}

          {!hasTelemetry && (
            <div className={styles.telemetryHint}>Awaiting telemetry data</div>
          )}
        </div>
      )}

      <Handle
        type="source"
        position={sourceP}
        className={styles.handle}
      />
    </div>
  );
}

function formatDependencySummary(counts: DependencyLinkCounts | undefined): string | undefined {
  if (!counts || (counts.dependencies === 0 && counts.dependents === 0)) return undefined;
  const summary: string[] = [];
  if (counts.dependencies > 0) {
    summary.push(`${counts.dependencies} ${counts.dependencies === 1 ? "dependency" : "dependencies"}`);
  }
  if (counts.dependents > 0) {
    summary.push(`${counts.dependents} ${counts.dependents === 1 ? "dependent" : "dependents"}`);
  }
  return summary.join(" · ");
}

export const EntityNode = memo(EntityNodeComponent);
