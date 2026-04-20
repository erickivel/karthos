import { useQuery } from "@tanstack/react-query";
import { getStatus } from "@/api/discovery";
import { useAppStore } from "@/stores/useAppStore";
import { useEffect } from "react";
import type { SystemStatus } from "@/types/status";
import { useConnectionState } from "./useConnectionState";

function systemStatusConnectionState(data: SystemStatus) {
  return data.system_status === "nominal"
    ? "connected"
    : data.system_status === "degraded"
      ? "degraded"
      : "disconnected";
}

export function useSystemStatus() {
  const setConnectionStatus = useAppStore((s) => s.setConnectionStatus);

  const query = useQuery({
    queryKey: ["system-status"],
    queryFn: getStatus,
    refetchInterval: 30_000,
  });
  const connectionState = useConnectionState(
    query,
    systemStatusConnectionState,
  );

  useEffect(() => {
    if (connectionState) {
      setConnectionStatus(connectionState);
    }
  }, [connectionState, setConnectionStatus]);

  return query;
}
