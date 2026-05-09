import { memo, type CSSProperties } from "react";
import { type Node, type NodeProps } from "@xyflow/react";
import { Box, Boxes, Server } from "lucide-react";
import type { EntityKind, GraphNode } from "@/types/graph";
import { topologyGroupTestId } from "../testIds";
import styles from "./NamespaceNode.module.css";

interface NamespaceData extends GraphNode {
  nsWidth: number;
  nsHeight: number;
  variant?: EntityKind;
  accentColor?: string;
}

type NamespaceNodeData = NamespaceData & Record<string, unknown>;

function NamespaceNodeComponent({ data }: NodeProps<Node<NamespaceNodeData>>) {
  const node = data;
  const width = node.nsWidth ?? 200;
  const height = node.nsHeight ?? 80;
  const variant = node.variant === "node" || node.variant === "pod"
    ? node.variant
    : "namespace";
  const Icon = variant === "node" ? Server : variant === "pod" ? Box : Boxes;
  const containerClass = [
    styles.container,
    variant === "node" ? styles.nodeVariant : "",
    variant === "pod" ? styles.podVariant : "",
  ]
    .filter(Boolean)
    .join(" ");
  const pillClass = [
    styles.pill,
    variant === "node" ? styles.pillNode : "",
    variant === "pod" ? styles.pillPod : "",
  ]
    .filter(Boolean)
    .join(" ");

  // For pod rectangles inside the Infrastructure "Containers" view we tint
  // the rectangle + pill by the pod's namespace so visually-adjacent pods
  // from the same namespace read as a group.
  const accent = variant === "pod" ? node.accentColor : undefined;
  const containerStyle: CSSProperties = { width, height };
  if (accent) {
    containerStyle.background = `color-mix(in srgb, ${accent} 12%, transparent)`;
    containerStyle.borderColor = `color-mix(in srgb, ${accent} 40%, transparent)`;
  }
  const pillStyle: CSSProperties | undefined = accent
    ? { background: accent, color: "var(--surface-base)" }
    : undefined;

  return (
    <div
      className={containerClass}
      style={containerStyle}
      data-testid={topologyGroupTestId(variant, node.name)}
      data-group-kind={variant}
      data-group-name={node.name}
    >
      <div className={pillClass} style={pillStyle}>
        <Icon size={12} strokeWidth={1.5} className={styles.pillIcon} />
        <span className={styles.pillLabel}>{node.name}</span>
      </div>
    </div>
  );
}

export const NamespaceNode = memo(NamespaceNodeComponent);
