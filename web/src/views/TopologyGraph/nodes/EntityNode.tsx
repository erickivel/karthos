import { memo, type KeyboardEvent, type MouseEvent } from "react";
import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import {
  Globe,
  Boxes,
  Box,
  Server,
  Container,
  CircleDot,
  ArrowRightFromLine,
  FileText,
  Lock,
  HardDrive,
  type LucideIcon,
} from "lucide-react";
import { SegmentedBar } from "@/components/shared/SegmentedBar";
import { useAppStore } from "@/stores/useAppStore";
import type { GraphNode, EntityKind } from "@/types/graph";
import { formatCpu, formatMem } from "../../../utils/formatters";
import { topologyEntityTestId } from "../testIds";
import styles from "./EntityNode.module.css";

type GraphNodeData = GraphNode & Record<string, unknown>;

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

function getHeaderColor(kind: EntityKind, status: string): string {
  if (status === "error") return "var(--color-alert)";
  return KIND_COLOR_VAR[kind] ?? "var(--surface-active)";
}

const SMALL_KINDS: EntityKind[] = ["configmap", "secret", "pvc"];

const KIND_ICONS: Record<EntityKind, LucideIcon> = {
  cluster: Boxes,
  service: Globe,
  workload: Boxes,
  pod: Box,
  node: Server,
  namespace: Boxes,
  container: Container,
  ingress: ArrowRightFromLine,
  endpoint: CircleDot,
  configmap: FileText,
  secret: Lock,
  pvc: HardDrive,
};

interface MetricBarData {
  label: string;
  value: number;
  max: number;
  unit?: string;
}

function getMetricBars(node: GraphNode): MetricBarData[] {
  const bars: MetricBarData[] = [];
  const labels = node.labels ?? {};

  const cpuRaw = labels["cpu_usage_millicores"];
  if (cpuRaw) {
    const cpu = parseFloat(cpuRaw);
    const maxCpu = parseFloat(labels["cpu_capacity_millicores"] || "100");
    bars.push({
      label: "CPU",
      value: cpu,
      max: maxCpu,
      unit: "m",
    });
  }

  const memRaw = labels["memory_usage_bytes"];
  if (memRaw) {
    const mem = parseFloat(memRaw);
    const maxMem = parseFloat(
      labels["memory_capacity_bytes"] || String(256 * 1024 * 1024),
    );
    bars.push({
      label: "MEM",
      value: mem,
      max: maxMem,
    });
  }

  return bars;
}

function EntityNodeComponent({ data }: NodeProps<Node<GraphNodeData>>) {
  const node = data;
  const setSelectedEntityId = useAppStore((s) => s.setSelectedEntityId);
  const isSmall = SMALL_KINDS.includes(node.kind);
  const hasTelemetry =
    node.status !== "unknown" || Object.keys(node.labels ?? {}).length > 0;
  const IconComponent = KIND_ICONS[node.kind] ?? Box;
  const metricBars = getMetricBars(node);
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
      onClick={handleClick}
      onKeyDown={handleKeyDown}
      role="button"
      tabIndex={0}
    >
      <Handle type="target" position={Position.Top} className={styles.handle} />

      <div
        className={styles.header}
        style={{ backgroundColor: getHeaderColor(node.kind, node.status) }}
      >
        <span className={styles.name}>{node.name}</span>
        <IconComponent size={14} strokeWidth={1.5} className={styles.icon} />
      </div>

      <div className={styles.body}>
        <div className={styles.prop}>
          <span className={styles.propLabel}>KIND</span>
          <span className={styles.propValue}>{node.kind}</span>
        </div>
        {node.namespace && (
          <div className={styles.prop}>
            <span className={styles.propLabel}>NS</span>
            <span className={styles.propValue}>{node.namespace}</span>
          </div>
        )}
      </div>

      {metricBars.length > 0 && (
        <div className={styles.metrics} data-topology-dynamic="metrics">
          {metricBars.map((bar) => (
            <div key={bar.label} className={styles.metricBlock}>
              <div className={styles.metricHeader}>
                <span className={styles.metricLabel}>{bar.label}</span>
                <span className={styles.metricValue}>
                  {bar.label === "CPU"
                    ? formatCpu(bar.value)
                    : formatMem(bar.value)}
                </span>
              </div>
              <SegmentedBar value={bar.value} max={bar.max} unit={bar.unit} />
            </div>
          ))}
        </div>
      )}

      {!hasTelemetry && (
        <div className={styles.telemetryHint}>Awaiting telemetry data</div>
      )}

      <Handle
        type="source"
        position={Position.Bottom}
        className={styles.handle}
      />
    </div>
  );
}

export const EntityNode = memo(EntityNodeComponent);
