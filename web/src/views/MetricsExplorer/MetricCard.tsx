import {
  BarChart,
  Bar,
  XAxis,
  ResponsiveContainer,
  Cell,
  Tooltip,
} from "recharts";
import type { MetricSample } from "@/types/metrics";
import { formatValue } from "../../utils/formatters";
import styles from "./MetricCard.module.css";

interface MetricCardProps {
  name: string;
  id?: string;
  unit: string;
  samples: MetricSample[];
}

interface TooltipPayloadEntry {
  value: number;
  payload: { time: string; fullTime: string; value: number };
}

function CustomTooltip({
  active,
  payload,
  unit,
}: {
  active?: boolean;
  payload?: TooltipPayloadEntry[];
  unit: string;
}) {
  if (!active || !payload || payload.length === 0) return null;
  const entry = payload[0]!;
  return (
    <div className={styles.tooltip}>
      <span className={styles.tooltipValue}>
        {formatValue(entry.value)} {unit}
      </span>
      <span className={styles.tooltipTime}>{entry.payload.fullTime}</span>
    </div>
  );
}

export function MetricCard({ name, id, unit, samples }: MetricCardProps) {
  const sorted = [...samples].sort(
    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
  );

  const currentValue = sorted.length > 0 ? sorted[sorted.length - 1]!.value : 0;

  const chartData = sorted.map((s, i) => ({
    time: new Date(s.timestamp).toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
    }),
    fullTime: new Date(s.timestamp).toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    }),
    value: s.value,
    index: i,
  }));

  return (
    <div className={styles.card}>
      <div className={styles.header}>
        <div className={styles.heading}>
          <span className={styles.name}>{name}</span>
          {id && <span className={styles.subtitle}>{id}</span>}
        </div>
        <span className={styles.currentValue}>
          {formatValue(currentValue)}{" "}
          <span className={styles.unit}>{unit}</span>
        </span>
      </div>

      {chartData.length > 0 ? (
        <div className={styles.chart}>
          <ResponsiveContainer width="100%" height={100}>
            <BarChart data={chartData} barCategoryGap="20%">
              <XAxis
                dataKey="time"
                tick={{ fontSize: 9, fill: "var(--color-text-muted)" }}
                axisLine={false}
                tickLine={false}
              />
              <Tooltip
                content={<CustomTooltip unit={unit} />}
                cursor={{ fill: "rgba(255, 255, 255, 0.05)" }}
              />
              <Bar dataKey="value" radius={[1, 1, 0, 0]}>
                {chartData.map((entry) => (
                  <Cell
                    key={entry.index}
                    fill={`rgba(8, 220, 125, ${0.3 + (entry.index / chartData.length) * 0.7})`}
                  />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>
      ) : (
        <div className={styles.empty}>No samples</div>
      )}
    </div>
  );
}
