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
import { useGraphSnapshot } from "@/hooks/useGraphSnapshot";
import { useAppStore } from "@/stores/useAppStore";
import type { GraphNode } from "@/types/graph";
import { EdgeMetricButton } from "./EdgeMetricButton";
import { edgeMetricsUrl, entityMetricsUrl } from "@/lib/links";
import styles from "./NodeInspector.module.css";

interface InfoItem {
  label: string;
  value: string;
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
      <span className={styles.dataKey} title={item.label}>
        {item.label}
      </span>
      <strong className={styles.dataValue} title={item.value}>
        {item.value}
      </strong>
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
  const { data: graphSnapshot } = useGraphSnapshot();

  const snapshotDetail = useMemo(() => {
    if (!selectedEntityId || !graphSnapshot) return undefined;
    const nodeById = new Map(graphSnapshot.graph.nodes.map((node) => [node.id, node]));
    const node = nodeById.get(selectedEntityId);
    if (!node) return undefined;
    return {
      node,
      inbound: graphSnapshot.graph.edges
        .filter((edge) => edge.target === selectedEntityId)
        .map((edge) => ({ edge, relatedNode: nodeById.get(edge.source) })),
      outbound: graphSnapshot.graph.edges
        .filter((edge) => edge.source === selectedEntityId)
        .map((edge) => ({ edge, relatedNode: nodeById.get(edge.target) })),
    };
  }, [selectedEntityId, graphSnapshot]);

  if (!selectedEntityId) return null;

  if (!snapshotDetail) return null;

  const { node, inbound, outbound } = snapshotDetail;

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
          {inbound.length > 0 ? (
            <div className={styles.edgeList}>
              {inbound.map(({ edge, relatedNode }) => {
                const relatedName = relatedNode?.name || String(edge.source);
                return (
                  <div key={edge.id} className={styles.edgeItem}>
                    <button
                      className={styles.edgePrimary}
                      onClick={() =>
                        relatedNode && setSelectedEntityId(relatedNode.id)
                      }
                      aria-label={`Select inbound ${relatedName}`}
                    >
                      <span className={styles.directionIcon} data-direction="inbound">
                        <ArrowDown size={14} strokeWidth={2} />
                      </span>
                      <span className={styles.edgeEntity}>{relatedName}</span>
                      <span className={styles.edgeRelation}>{edge.relation}</span>
                    </button>
                    <EdgeMetricButton edge={edge} onNavigate={openEdgeMetrics} />
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
          {outbound.length > 0 ? (
            <div className={styles.edgeList}>
              {outbound.map(({ edge, relatedNode }) => {
                const relatedName = relatedNode?.name || String(edge.target);
                return (
                  <div key={edge.id} className={styles.edgeItem}>
                    <button
                      className={styles.edgePrimary}
                      onClick={() =>
                        relatedNode && setSelectedEntityId(relatedNode.id)
                      }
                      aria-label={`Select outbound ${relatedName}`}
                    >
                      <span className={styles.directionIcon} data-direction="outbound">
                        <ArrowUp size={14} strokeWidth={2} />
                      </span>
                      <span className={styles.edgeEntity}>{relatedName}</span>
                      <span className={styles.edgeRelation}>{edge.relation}</span>
                    </button>
                    <EdgeMetricButton edge={edge} onNavigate={openEdgeMetrics} />
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
