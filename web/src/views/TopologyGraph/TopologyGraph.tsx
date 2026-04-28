import { useCallback, useMemo } from "react";
import { useSearchParams } from "react-router-dom";
import {
  ReactFlow,
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
import { useGraphLayout } from "./useGraphLayout";
import { useProjectionState } from "./useProjectionState";
import { project } from "./projection";
import { EntityNode } from "./nodes/EntityNode";
import { NamespaceNode } from "./nodes/NamespaceNode";
import { RelationEdge } from "./edges/RelationEdge";
import { FilterToolbar } from "./FilterToolbar";
import { NodeInspector } from "./NodeInspector";
import styles from "./TopologyGraph.module.css";

const nodeTypes = { entity: EntityNode, namespace: NamespaceNode };
const edgeTypes = { relation: RelationEdge };

export function TopologyGraph() {
  const { data: snapshot } = useGraphSnapshot();
  const nodeMetrics = useNodeMetrics();
  const edgeMetrics = useEdgeMetrics();
  const [searchParams] = useSearchParams();
  const setSelectedEntityId = useAppStore((s) => s.setSelectedEntityId);
  const { mode, granularity, includeOrphans } = useProjectionState();

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

  const { nodes, edges } = useGraphLayout(projection, mode, nodeMetrics, edgeMetrics);

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
      <div className={styles.graphSurface} data-testid="topology-graph">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          onNodeClick={onNodeClick}
          onPaneClick={onPaneClick}
          fitView
          fitViewOptions={{ padding: 0.26 }}
          minZoom={0.1}
          maxZoom={2}
          proOptions={{ hideAttribution: true }}
        >
          <Background
            variant={BackgroundVariant.Dots}
            gap={24}
            size={1.5}
            bgColor="transparent"
          />
          <MiniMap
            nodeColor="var(--surface-active)"
            maskColor="rgba(18, 20, 22, 0.8)"
            style={{ background: "var(--surface-nav)" }}
          />
          <Controls
            showInteractive={false}
            style={{ background: "var(--surface-nav)" }}
          />
        </ReactFlow>
      </div>
      <FilterToolbar filters={snapshot?.summary.available_filters} />
      <NodeInspector />
    </div>
  );
}
