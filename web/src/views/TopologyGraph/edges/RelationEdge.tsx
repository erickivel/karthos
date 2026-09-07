import { memo, useLayoutEffect, useRef, useState } from "react";
import { getSmoothStepPath, type EdgeProps } from "@xyflow/react";
import type { GraphEdge, RelationType } from "@/types/graph";
import styles from "./RelationEdge.module.css";

interface RoutedPoint {
  x: number;
  y: number;
}

type RelationEdgeData = GraphEdge & {
    routePoints?: RoutedPoint[];
    routeStyle?: "ownership" | "orthogonal";
    rawElk?: boolean;
  };

function getEdgeColor(relation: RelationType): string {
  switch (relation) {
    case "calls":
      return "var(--color-signal)";
    case "exposes":
      // Structural in Workload Flow — muted to read distinct from `calls`,
      // which carries traffic-rate styling.
      return "var(--color-text-muted)";
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

function getRoundedRoutePath(points: RoutedPoint[], radius = 10): string {
  const deduped = points.filter((point, index) => {
    const previous = points[index - 1];
    return !previous || previous.x !== point.x || previous.y !== point.y;
  });

  if (deduped.length < 2) return "";

  const first = deduped[0]!;
  const commands = [`M ${first.x} ${first.y}`];
  for (let index = 1; index < deduped.length - 1; index++) {
    const previous = deduped[index - 1]!;
    const current = deduped[index]!;
    const next = deduped[index + 1]!;
    const incoming = {
      x: current.x - previous.x,
      y: current.y - previous.y,
    };
    const outgoing = {
      x: next.x - current.x,
      y: next.y - current.y,
    };
    const incomingLength = Math.hypot(incoming.x, incoming.y);
    const outgoingLength = Math.hypot(outgoing.x, outgoing.y);
    const isCorner =
      incomingLength > 0 &&
      outgoingLength > 0 &&
      Math.abs(incoming.x * outgoing.x + incoming.y * outgoing.y) < 0.01;

    if (!isCorner) {
      commands.push(`L ${current.x} ${current.y}`);
      continue;
    }

    const cornerRadius = Math.min(radius, incomingLength / 2, outgoingLength / 2);
    const before = {
      x: current.x - (incoming.x / incomingLength) * cornerRadius,
      y: current.y - (incoming.y / incomingLength) * cornerRadius,
    };
    const after = {
      x: current.x + (outgoing.x / outgoingLength) * cornerRadius,
      y: current.y + (outgoing.y / outgoingLength) * cornerRadius,
    };

    commands.push(`L ${before.x} ${before.y}`);
    commands.push(`Q ${current.x} ${current.y} ${after.x} ${after.y}`);
  }

  const last = deduped[deduped.length - 1]!;
  commands.push(`L ${last.x} ${last.y}`);
  return commands.join(" ");
}

function getRoutedPoints(
  edge: RelationEdgeData | undefined,
  routePoints: RoutedPoint[] | null,
  props: EdgeProps,
): RoutedPoint[] | null {
  // ELK-routed edges: render the engine's section points as-is.
  // Live React Flow handle coordinates are intentionally ignored here; rewriting
  // the first/last segment from sourceX/targetX would create the diagonal
  // artifact described in docs/topology-elk-raw-react-flow.md.
  if (edge?.rawElk && routePoints) return routePoints;

  if (edge?.routeStyle !== "ownership") return null;

  if (routePoints) {
    return routePoints;
  }

  const source = { x: props.sourceX, y: props.sourceY };
  const target = { x: props.targetX, y: props.targetY };
  const midY = (source.y + target.y) / 2;
  return [source, { x: source.x, y: midY }, { x: target.x, y: midY }, target];
}

function RelationEdgeComponent(props: EdgeProps) {
  const edge = props.data as unknown as RelationEdgeData | undefined;
  const relation = edge?.relation ?? "calls";
  const directed = DIRECTED_RELATIONS.includes(relation);
  const showLabel = !(relation === "contains" && edge?.routeStyle === "ownership");
  const routePoints =
    edge?.routePoints && edge.routePoints.length >= 2 ? edge.routePoints : null;

  const [smoothPath, fallbackLabelX, fallbackLabelY] = getSmoothStepPath({
    sourceX: props.sourceX,
    sourceY: props.sourceY,
    targetX: props.targetX,
    targetY: props.targetY,
    sourcePosition: props.sourcePosition,
    targetPosition: props.targetPosition,
    borderRadius: 8,
  });
  const explicitRoutePoints = getRoutedPoints(edge, routePoints, props);
  const edgePath = explicitRoutePoints
    ? getRoundedRoutePath(explicitRoutePoints)
    : smoothPath;

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

  return (
    <>
      <path
        ref={measureRef}
        d={edgePath}
        fill="none"
        stroke="var(--surface-active)"
        strokeWidth={3}
        strokeOpacity={0.95}
        strokeLinecap="round"
      />
      <path
        d={edgePath}
        fill="none"
        stroke={getEdgeColor(relation)}
        strokeWidth={1.35}
        strokeOpacity={0.7}
        strokeLinecap="round"
        strokeDasharray={relation === 'exposes' ? '4 3' : undefined}
        markerEnd={directed ? props.markerEnd : undefined}
      />
      {showLabel && <foreignObject
        x={labelX - 68}
        y={labelY - 18}
        width={136}
        height={36}
        className={styles.labelWrapper}
        data-testid={`topology-edge-${relation}`}
      >
        <div className={styles.label}>
          <span>
            {relation}
            {getDirectionSymbol(relation)}
          </span>
        </div>
      </foreignObject>}
    </>
  );
}

export const RelationEdge = memo(RelationEdgeComponent);
