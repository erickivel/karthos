import type { UseQueryResult } from "@tanstack/react-query";

type ConnectionState = "connected" | "degraded" | "disconnected";

export function useConnectionState<TData>(
  query: Pick<UseQueryResult<TData>, "data" | "isError">,
  fromData?: (data: TData) => ConnectionState,
): ConnectionState | null {
  if (query.isError) return "disconnected";
  if (!query.data) return null;
  return fromData ? fromData(query.data) : "connected";
}
