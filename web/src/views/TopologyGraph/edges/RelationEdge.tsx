import { memo, useLayoutEffect, useRef, useState } from "react";
import { getSmoothStepPath, type EdgeProps } from "@xyflow/react";
import type { GraphEdge, RelationType } from "@/types/graph";
import { formatLatency } from "../../../utils/formatters";
import type { GraphEdgeMetricData } from "../edgeMetrics";
import styles from "./RelationEdge.module.css";

function getEdgeColor(relation: RelationType): string {
  switch (relation) {
    case "calls":
    case "exposes":
      return "var(--color-signal)";
    case "runs_on":
    case "contains":
      return "var(--color-info)";
    default:
      return "var(--color-text-muted)";
  }
}

// Relations where direction is meaningful and should show an arrow
const DIRECTED_RELATIONS: RelationType[] = [
  "calls",
  "runs_on",
  "routes_to",
  "exposes",
];

function getDirectionSymbol(relation: RelationType): string {
  if (DIRECTED_RELATIONS.includes(relation)) return " \u2192";
  return "";
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}

function getCableOffsets(count: number, spacing: number): number[] {
  const center = (count - 1) / 2;
  return Array.from(
    { length: count },
    (_, index) => (index - center) * spacing,
  );
}

function getMetricState(edge: GraphEdge & GraphEdgeMetricData) {
  const rate = edge.metrics?.requestRate ?? 0;
  const latencyNs = edge.metrics?.latencyNs;
  const errorRatio = edge.metrics?.errorRatio ?? 0;
  const errorRate = edge.metrics?.errorRate ?? 0;

  const cableCount =
    edge.relation === "calls"
      ? Math.min(6, Math.max(1, Math.ceil(Math.log10(rate + 1) * 2.4)))
      : 1;
  const cableWidth = 1.15 + clamp(Math.log10(rate + 1) * 0.24, 0, 0.7);
  const cableSpacing =
    cableCount > 1 ? 2 + clamp(Math.log10(rate + 1) * 0.35, 0, 0.9) : 0;
  const bundleWidth =
    cableWidth + cableSpacing * Math.max(0, cableCount - 1) + 2;
  const opacity = 0.5 + clamp(Math.log10(rate + 1) * 0.1, 0, 0.22);
  const latencyMs = latencyNs !== undefined ? latencyNs / 1_000_000 : 0;

  let stroke = getEdgeColor(edge.relation);
  if (errorRatio >= 0.1 || errorRate >= 1) {
    stroke = "var(--color-alert)";
  } else if (latencyMs >= 300) {
    stroke = "var(--color-warn)";
  } else if (latencyMs >= 120) {
    stroke = "var(--color-info)";
  }

  return {
    cableCount,
    cableWidth,
    cableSpacing,
    bundleWidth,
    opacity: clamp(opacity, 0.38, 0.88),
    stroke,
    summary: [
      rate > 0 ? `${rate.toFixed(rate >= 10 ? 0 : 1)} rps` : null,
      formatLatency(latencyNs),
      errorRatio > 0 ? `${(errorRatio * 100).toFixed(1)}% err` : null,
    ]
      .filter(Boolean)
      .join(" · "),
  };
}

function RelationEdgeComponent(props: EdgeProps) {
  const edge = props.data as unknown as
    | (GraphEdge & GraphEdgeMetricData)
    | undefined;
  const relation = edge?.relation ?? "calls";
  const directed = DIRECTED_RELATIONS.includes(relation);
  const metricState = edge ? getMetricState(edge) : null;
  const dx = props.targetX - props.sourceX;
  const dy = props.targetY - props.sourceY;
  const length = Math.hypot(dx, dy) || 1;
  const normalX = -dy / length;
  const normalY = dx / length;

  const [edgePath, fallbackLabelX, fallbackLabelY] = getSmoothStepPath({
    sourceX: props.sourceX,
    sourceY: props.sourceY,
    targetX: props.targetX,
    targetY: props.targetY,
    sourcePosition: props.sourcePosition,
    targetPosition: props.targetPosition,
    borderRadius: 8,
  });

  // True midpoint of the rendered SVG path, computed via getPointAtLength.
  // Falls back to xyflow's labelX/Y until the path mounts.
  const measureRef = useRef<SVGPathElement>(null);
  const [midpoint, setMidpoint] = useState<{ x: number; y: number } | null>(null);
  useLayoutEffect(() => {
    const el = measureRef.current;
    if (!el) return;
    const total = el.getTotalLength();
    if (total === 0) return;
    const pt = el.getPointAtLength(total / 2);
    setMidpoint((prev) => {
      if (prev && Math.abs(prev.x - pt.x) < 0.5 && Math.abs(prev.y - pt.y) < 0.5) {
        return prev;
      }
      return { x: pt.x, y: pt.y };
    });
  }, [edgePath]);
  const labelX = midpoint?.x ?? fallbackLabelX;
  const labelY = midpoint?.y ?? fallbackLabelY;

  const cableOffsets = getCableOffsets(
    metricState?.cableCount ?? 1,
    metricState?.cableSpacing ?? 0,
  );
  const markerCableIndex = Math.floor(cableOffsets.length / 2);

  return (
    <>
      <path
        ref={measureRef}
        d={edgePath}
        fill="none"
        stroke="var(--surface-active)"
        strokeWidth={metricState?.bundleWidth ?? 3}
        strokeOpacity={0.95}
        strokeLinecap="round"
      />
      {cableOffsets.map((offset, index) => (
        <path
          key={`${props.id}-cable-${index}`}
          d={edgePath}
          fill="none"
          stroke={metricState?.stroke ?? getEdgeColor(relation)}
          strokeWidth={metricState?.cableWidth ?? 1.35}
          strokeOpacity={
            metricState
              ? Math.max(0.42, metricState.opacity - index * 0.03)
              : 0.7
          }
          strokeLinecap="round"
          transform={
            offset === 0
              ? undefined
              : `translate(${(normalX * offset).toFixed(2)} ${(normalY * offset).toFixed(2)})`
          }
          markerEnd={
            directed && index === markerCableIndex ? props.markerEnd : undefined
          }
        />
      ))}
      <foreignObject
        x={labelX - 68}
        y={labelY - 18}
        width={136}
        height={36}
        className={styles.labelWrapper}
      >
        <div className={styles.label}>
          <span>
            {relation}
            {getDirectionSymbol(relation)}
          </span>
          {metricState?.summary && (
            <span className={styles.metricSummary}>{metricState.summary}</span>
          )}
        </div>
      </foreignObject>
    </>
  );
}

export const RelationEdge = memo(RelationEdgeComponent);
