import type { GraphEdge } from "@/types/graph";
import styles from "./NodeInspector.module.css";

interface EdgeMetricButtonProps {
  edge: GraphEdge;
  onNavigate: (sourceId: string, targetId: string) => void;
}

export function EdgeMetricButton({ edge, onNavigate }: EdgeMetricButtonProps) {
  if (edge.relation !== "calls") return null;

  return (
    <button
      className={styles.edgeMetricLink}
      onClick={() => onNavigate(String(edge.source), String(edge.target))}
    >
      Metrics
    </button>
  );
}
