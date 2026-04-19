export function formatCpu(cpu: number): string {
  if (cpu >= 1000) return `${(cpu / 1000).toFixed(1)}`;
  if (cpu >= 1) return `${cpu.toFixed(1)}m`;
  return `${(cpu * 1000).toFixed(0)}µ`;
}

export function formatMem(mem: number): string {
  if (mem >= 1_073_741_824) return `${(mem / 1_073_741_824).toFixed(1)}G`;
  if (mem >= 1_048_576) return `${(mem / 1_048_576).toFixed(0)}M`;
  if (mem >= 1024) return `${(mem / 1024).toFixed(0)}K`;
  return `${mem.toFixed(0)}B`;
}

export function formatBytes(n: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = n;
  let index = 0;

  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index++;
  }

  return `${value.toFixed(value >= 100 || index === 0 ? 0 : 1)} ${units[index]}`;
}

export function formatLatency(ns?: number): string | null {
  if (ns === undefined) return null;
  const ms = ns / 1_000_000;
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
  if (ms >= 100) return `${Math.round(ms)}ms`;
  if (ms >= 10) return `${ms.toFixed(1)}ms`;
  return `${ms.toFixed(2)}ms`;
}

export function formatAge(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s AGO`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m AGO`;
  return `${Math.round(seconds / 3600)}h AGO`;
}

export function formatValue(v: number): string {
  if (v >= 1_000_000_000) return `${(v / 1_000_000_000).toFixed(2)}G`;
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(2)}M`;
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}K`;
  return v.toFixed(v % 1 === 0 ? 0 : 2);
}
