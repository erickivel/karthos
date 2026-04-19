import { useQuery } from "@tanstack/react-query";
import { getSnapshot } from "@/api/graph";
import { useAppStore } from "@/stores/useAppStore";
import { useEffect } from "react";
import type { GraphSnapshot } from "@/types/graph";
import { useConnectionState } from "./useConnectionState";
import { usePollingEnabled } from "./usePollingEnabled";

/** Ensure every node has top-level name/namespace (fall back to labels). */
function normalizeSnapshot(snapshot: GraphSnapshot): GraphSnapshot {
  return {
    ...snapshot,
    graph: {
      ...snapshot.graph,
      nodes: snapshot.graph.nodes.map((n) => ({
        ...n,
        labels: n.labels ?? {},
        name: n.name || n.labels?.["name"] || n.id,
        namespace: n.namespace || n.labels?.["namespace"] || "",
        cluster: n.cluster || "",
        status: n.status || "unknown",
      })),
    },
  };
}

export function useGraphSnapshot() {
  const setConnectionStatus = useAppStore((s) => s.setConnectionStatus);
  const pollingEnabled = usePollingEnabled();

  const query = useQuery({
    queryKey: ["graph-snapshot"],
    queryFn: getSnapshot,
    refetchInterval: pollingEnabled ? 30_000 : false,
    select: normalizeSnapshot,
  });
  const connectionState = useConnectionState(query);

  useEffect(() => {
    if (connectionState === "disconnected") {
      setConnectionStatus("disconnected");
    }
  }, [connectionState, setConnectionStatus]);

  return query;
}
