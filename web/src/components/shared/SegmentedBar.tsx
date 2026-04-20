import { formatValue } from "../../utils/formatters";
import styles from "./SegmentedBar.module.css";

interface SegmentedBarProps {
  value: number;
  max: number;
  segments?: number;
  label?: string;
  unit?: string;
}

export function SegmentedBar({
  value,
  max,
  segments = 10,
  label,
  unit,
}: SegmentedBarProps) {
  const ratio = max > 0 ? Math.min(value / max, 1) : 0;
  const filledCount = Math.round(ratio * segments);
  const formattedValue = formatValue(value);

  return (
    <div className={styles.container}>
      {label && (
        <div className={styles.header}>
          <span className={styles.label}>{label}</span>
          <span className={styles.value}>
            {formattedValue}
            {unit ? ` ${unit}` : ""}
          </span>
        </div>
      )}
      <div className={styles.bar}>
        {Array.from({ length: segments }, (_, i) => (
          <div
            key={i}
            className={styles.segment}
            data-filled={i < filledCount}
          />
        ))}
      </div>
    </div>
  );
}
