import { describe, expect, it } from "vitest";
import {
  formatAge,
  formatBytes,
  formatCpu,
  formatLatency,
  formatMetricValue,
  formatMem,
  formatValue,
} from "./formatters";

describe("formatters", () => {
  it("formats cpu values across micro, milli, and core ranges", () => {
    expect(formatCpu(0.25)).toBe("250µ");
    expect(formatCpu(42)).toBe("42.0m");
    expect(formatCpu(1500)).toBe("1.5");
  });

  it("formats memory values for compact topology labels", () => {
    expect(formatMem(512)).toBe("512B");
    expect(formatMem(2048)).toBe("2K");
    expect(formatMem(5 * 1_048_576)).toBe("5M");
    expect(formatMem(3 * 1_073_741_824)).toBe("3.0G");
  });

  it("formats bytes for inspector summaries", () => {
    expect(formatBytes(999)).toBe("999 B");
    expect(formatBytes(1_536)).toBe("1.5 KB");
    expect(formatBytes(5 * 1_048_576)).toBe("5.0 MB");
  });

  it("formats latency from nanoseconds through seconds", () => {
    expect(formatLatency()).toBeNull();
    expect(formatLatency(9_500_000)).toBe("9.50ms");
    expect(formatLatency(125_000_000)).toBe("125ms");
    expect(formatLatency(1_500_000_000)).toBe("1.5s");
  });

  it("separates a human-readable duration value from its unit", () => {
    expect(formatMetricValue(5_625_000, "nanoseconds")).toEqual({ value: "5.63", unit: "ms" });
    expect(formatMetricValue(830_000, "nanoseconds")).toEqual({ value: "0.83", unit: "ms" });
    expect(formatMetricValue(1_500_000_000, "nanoseconds")).toEqual({ value: "1.5", unit: "s" });
  });

  it("formats snapshot ages in status-bar style", () => {
    expect(formatAge(45)).toBe("45s AGO");
    expect(formatAge(120)).toBe("2m AGO");
    expect(formatAge(7_200)).toBe("2h AGO");
  });

  it("formats generic numeric values for charts and segmented bars", () => {
    expect(formatValue(12)).toBe("12");
    expect(formatValue(12.34)).toBe("12.34");
    expect(formatValue(1_250)).toBe("1.3K");
    expect(formatValue(2_500_000)).toBe("2.50M");
  });
});
