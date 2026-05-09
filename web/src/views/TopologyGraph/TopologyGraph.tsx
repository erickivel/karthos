import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import {
  ReactFlow,
  ReactFlowProvider,
  MiniMap,
  Controls,
  Background,
  BackgroundVariant,
  type NodeMouseHandler,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useGraphSnapshot } from "@/hooks/useGraphSnapshot";
import { useNodeMetrics } from "@/hooks/useNodeMetrics";
import { useEdgeMetrics } from "@/hooks/useEdgeMetrics";
import { useAppStore } from "@/stores/useAppStore";
import { useGraphLayout, type LayoutInput } from "./useGraphLayout";
import { useProjectionState } from "./useProjectionState";
import { project } from "./projection";
import type { MeasuredSize } from "./layouts/types";
import { EntityNode } from "./nodes/EntityNode";
import { NamespaceNode } from "./nodes/NamespaceNode";
import { RelationEdge } from "./edges/RelationEdge";
import { FilterToolbar } from "./FilterToolbar";
import { NodeInspector } from "./NodeInspector";
import styles from "./TopologyGraph.module.css";

const nodeTypes = { entity: EntityNode, namespace: NamespaceNode };
const edgeTypes = { relation: RelationEdge };

// MiniMap fills are read by SVG <rect> elements inside React Flow's MiniMap
// component. Returning the per-kind CSS variable lets each entity show its
// kind color in the minimap, and namespace/host/pod parent rectangles get a
// muted neutral so they read as containers, not entities.
function miniMapNodeColor(node: { type?: string; data?: Record<string, unknown> }): string {
  if (node.type === "namespace") {
    const variant = node.data?.variant;
    if (variant === "node") return "var(--kind-node)";
    if (variant === "pod") return "var(--kind-pod)";
    return "var(--kind-namespace)";
  }
  const kind = node.data?.kind as string | undefined;
  return kind ? `var(--kind-${kind})` : "var(--color-text-muted)";
}

function miniMapNodeStroke(node: { type?: string }): string {
  return node.type === "namespace"
    ? "rgba(255, 255, 255, 0.25)"
    : "rgba(255, 255, 255, 0.55)";
}

export function TopologyGraph() {
  return (
    <ReactFlowProvider>
      <TopologyGraphInner />
    </ReactFlowProvider>
  );
}

function TopologyGraphInner() {
  const { data: snapshot } = useGraphSnapshot();
  const nodeMetrics = useNodeMetrics();
  const edgeMetrics = useEdgeMetrics();
  const [searchParams] = useSearchParams();
  const setSelectedEntityId = useAppStore((s) => s.setSelectedEntityId);
  const { mode, granularity, includeOrphans, layoutAlgorithm } = useProjectionState();

  const kindFilter = searchParams.get("kind");
  const namespaceFilter = searchParams.get("namespace");
  const relationFilter = searchParams.get("relation");

  const filteredGraph = useMemo(() => {
    if (!snapshot) return { nodes: [], edges: [] };

    let nodes = snapshot.graph.nodes;
    let edges = snapshot.graph.edges;

    if (kindFilter) {
      nodes = nodes.filter(
        (n) =>
          n.kind === kindFilter ||
          (kindFilter !== "namespace" && n.kind === "namespace") ||
          (kindFilter !== "node" && n.kind === "node"),
      );
    }
    if (namespaceFilter) {
      const namespacedNodeIds = new Set(
        nodes
          .filter(
            (n) =>
              n.namespace === namespaceFilter ||
              (n.kind === "namespace" && n.name === namespaceFilter),
          )
          .map((n) => n.id),
      );
      const namespaceHostNodeIds = new Set(
        edges
          .filter((e) => e.relation === "runs_on" && namespacedNodeIds.has(e.source))
          .map((e) => e.target),
      );
      nodes = nodes.filter(
        (n) =>
          n.namespace === namespaceFilter ||
          (n.kind === "namespace" && n.name === namespaceFilter) ||
          (n.kind === "node" && namespaceHostNodeIds.has(n.id)),
      );
    }

    const nodeIds = new Set(nodes.map((n) => n.id));
    edges = edges.filter((e) => nodeIds.has(e.source) && nodeIds.has(e.target));

    if (relationFilter) {
      edges = edges.filter((e) => e.relation === relationFilter);
    }

    return { nodes, edges };
  }, [snapshot, kindFilter, namespaceFilter, relationFilter]);

  const projection = useMemo(
    () =>
      project(filteredGraph.nodes, filteredGraph.edges, mode, granularity, {
        includeOrphans,
      }),
    [filteredGraph, mode, granularity, includeOrphans],
  );

  // Measured node sizes harvested directly from the rendered React Flow nodes
  // via a ResizeObserver keyed on `data-id`. We deliberately do NOT use
  // `useReactFlow().getNodes()` / `useNodesInitialized` / `useStore(nodeLookup)`
  // — in v12 the user-facing node objects don't carry `measured`, and the
  // internal lookup wasn't surfacing measurements in this mounting topology.
  // The DOM observer is also robust to React Query snapshot polls: node IDs
  // are stable across polls so measured sizes persist; only IDs that genuinely
  // leave the DOM get evicted (via the MutationObserver branch below).
  const surfaceRef = useRef<HTMLDivElement | null>(null);
  const [measuredSizes, setMeasuredSizes] = useState<Map<string, MeasuredSize>>(
    () => new Map(),
  );
  useEffect(() => {
    const root = surfaceRef.current;
    if (!root) return;
    const sizes = new Map<string, MeasuredSize>(measuredSizes);
    let frame = 0;
    const flush = () => {
      frame = 0;
      setMeasuredSizes((prev) =>
        sizesEqual(prev, sizes) ? prev : new Map(sizes),
      );
    };
    const schedule = () => {
      if (frame) return;
      frame = window.requestAnimationFrame(flush);
    };
    const observer = new ResizeObserver((entries) => {
      let changed = false;
      for (const entry of entries) {
        const el = entry.target as HTMLElement;
        const id = el.getAttribute("data-id");
        if (!id) continue;
        const box = entry.borderBoxSize?.[0];
        const width = box ? box.inlineSize : entry.contentRect.width;
        const height = box ? box.blockSize : entry.contentRect.height;
        if (width <= 0 || height <= 0) continue;
        const previous = sizes.get(id);
        if (
          !previous ||
          Math.abs(previous.width - width) > 0.5 ||
          Math.abs(previous.height - height) > 0.5
        ) {
          sizes.set(id, { width, height });
          changed = true;
        }
      }
      if (changed) schedule();
    });
    const tracked = new Set<Element>();
    const sync = () => {
      const els = root.querySelectorAll<HTMLElement>(
        ".react-flow__node[data-id]",
      );
      const seen = new Set<Element>();
      els.forEach((el) => {
        seen.add(el);
        if (!tracked.has(el)) {
          tracked.add(el);
          observer.observe(el);
        }
      });
      for (const el of tracked) {
        if (!seen.has(el)) {
          tracked.delete(el);
          observer.unobserve(el);
          const id = el.getAttribute("data-id");
          if (id && sizes.delete(id)) schedule();
        }
      }
    };
    sync();
    const mutation = new MutationObserver(sync);
    mutation.observe(root, { childList: true, subtree: true });
    return () => {
      observer.disconnect();
      mutation.disconnect();
      if (frame) cancelAnimationFrame(frame);
    };
    // The observer instance is created once per mount; we intentionally do not
    // re-run when `measuredSizes` changes (the closure above mutates the same
    // Map reference and schedules a flush only on real diffs).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const layoutInput = useMemo<LayoutInput>(
    () => ({
      ...projection,
      nodeSizes: measuredSizes.size > 0 ? measuredSizes : undefined,
    }),
    [projection, measuredSizes],
  );

  const { nodes, edges } = useGraphLayout(
    layoutInput,
    mode,
    layoutAlgorithm,
    nodeMetrics,
    edgeMetrics,
  );

  const onNodeClick: NodeMouseHandler = useCallback(
    (_, node) => {
      setSelectedEntityId(node.id);
    },
    [setSelectedEntityId],
  );

  const onPaneClick = useCallback(() => {
    setSelectedEntityId(null);
  }, [setSelectedEntityId]);

  return (
    <div className={styles.container}>
      <div
        className={styles.graphSurface}
        data-testid="topology-graph"
        ref={surfaceRef}
      >
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          onNodeClick={onNodeClick}
          onPaneClick={onPaneClick}
          fitView
          fitViewOptions={{ padding: 0.14 }}
          minZoom={0.1}
          maxZoom={2}
          onlyRenderVisibleElements={false}
          proOptions={{ hideAttribution: true }}
        >
          <Background
            variant={BackgroundVariant.Dots}
            gap={24}
            size={1.5}
            bgColor="transparent"
          />
          {mode !== "traffic" && (
            // Traffic mode runs ELK; setting width/height on its nodes (which
            // MiniMap needs to render anything) breaks the measure→relayout
            // cycle and detaches edges from cards. Hide the minimap there
            // rather than ship a misleading empty box. See AGENTS.md.
            <MiniMap
              pannable
              zoomable
              nodeColor={miniMapNodeColor}
              nodeStrokeColor={miniMapNodeStroke}
              nodeStrokeWidth={2}
              nodeBorderRadius={3}
              maskColor="rgba(8, 9, 10, 0.72)"
              maskStrokeColor="rgba(255, 255, 255, 0.18)"
              maskStrokeWidth={1}
              className={styles.miniMap}
            />
          )}
          <Controls
            showInteractive={false}
            className={styles.controls}
          />
        </ReactFlow>
      </div>
      <FilterToolbar filters={snapshot?.summary.available_filters} />
      <NodeInspector />
    </div>
  );
}

function sizesEqual(
  prev: Map<string, MeasuredSize>,
  next: Map<string, MeasuredSize>,
): boolean {
  if (prev.size !== next.size) return false;
  for (const [id, size] of next) {
    const previous = prev.get(id);
    if (!previous) return false;
    if (Math.abs(previous.width - size.width) > 0.5) return false;
    if (Math.abs(previous.height - size.height) > 0.5) return false;
  }
  return true;
}
