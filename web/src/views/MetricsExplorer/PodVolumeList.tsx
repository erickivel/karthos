import { ChevronDown, HardDrive, Tag } from 'lucide-react'
import { formatMetricValue } from '@/utils/formatters'
import type { PodVolumeGroup } from './podVolumeModel'
import styles from './PodVolumeList.module.css'

interface Props { volumes: PodVolumeGroup[] }

function value(value?: { value: number; unit: string }) {
  if (!value) return 'No current sample'
  const displayed = formatMetricValue(value.value, value.unit)
  return `${displayed.value} ${displayed.unit}`.trim()
}

function labelsFor(volume: PodVolumeGroup) {
  return [volume.available, volume.capacity, volume.inodesFree, volume.inodesUsed, volume.inodes]
    .filter((sample): sample is NonNullable<typeof sample> => Boolean(sample))
    .flatMap((sample) => Object.entries(sample.labels).map(([key, value]) => `${key}=${value}`))
    .filter((value, index, values) => values.indexOf(value) === index)
    .sort()
}

/** Displays mount-level facts only; it never derives a Pod-wide storage total. */
export function PodVolumeList({ volumes }: Props) {
  if (volumes.length === 0) {
    return <div className={styles.empty}>No current volume samples for this Pod.</div>
  }

  return (
    <section className={styles.list} aria-label="Pod volumes">
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}><HardDrive size={13} aria-hidden="true" /> Pod volumes</span>
          <h2>Mount-level storage</h2>
        </div>
        <p>Each row is a separate mount. Values are never combined across volumes.</p>
      </header>
      <div className={styles.rows}>
        {volumes.map((volume) => {
          const labels = labelsFor(volume)
          return (
            <details key={volume.key} className={styles.row}>
              <summary>
                <span className={styles.name}>{volume.name}</span>
                <span className={styles.context}>
                  {volume.volumeType && <span>{volume.volumeType}</span>}
                  {volume.persistentVolumeClaimName && <span>claim: {volume.persistentVolumeClaimName}</span>}
                </span>
                <span className={styles.values}>
                  <span><small>Available</small>{value(volume.available)}</span>
                  <span><small>Capacity</small>{value(volume.capacity)}</span>
                </span>
                <ChevronDown className={styles.chevron} size={16} aria-hidden="true" />
              </summary>
              <div className={styles.detail}>
                <dl>
                  <div><dt>Free inodes</dt><dd>{value(volume.inodesFree)}</dd></div>
                  <div><dt>Used inodes</dt><dd>{value(volume.inodesUsed)}</dd></div>
                  <div><dt>Total inodes</dt><dd>{value(volume.inodes)}</dd></div>
                </dl>
                {labels.length > 0 && <p><Tag size={13} aria-hidden="true" /> {labels.join(' · ')}</p>}
              </div>
            </details>
          )
        })}
      </div>
    </section>
  )
}
