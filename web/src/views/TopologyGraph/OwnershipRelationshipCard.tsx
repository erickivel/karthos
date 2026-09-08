import { useEffect, useMemo, useRef, useState, type PointerEvent } from 'react'
import { GripVertical, Pin, PinOff, X } from 'lucide-react'
import { useGraphSnapshot } from '@/hooks/useGraphSnapshot'
import { useAppStore } from '@/stores/useAppStore'
import type { ProjectionMode } from './projection'
import { relationshipEntriesFor, type RelationshipEntry } from './ownershipRelationshipCardModel'
import styles from './OwnershipRelationshipCard.module.css'

// Keep the card clear of the filter toolbar on first open. It is still a
// canvas-local panel, so its position must be constrained against this graph
// surface rather than the browser window.
const DEFAULT_POSITION = { x: 28, y: 212 }

interface DragStart {
  pointerX: number
  pointerY: number
  cardX: number
  cardY: number
}

export function OwnershipRelationshipCard({ mode }: { mode: ProjectionMode }) {
  const { data: snapshot } = useGraphSnapshot()
  const cardEntityID = useAppStore((state) => state.relationshipCardEntityId)
  const pinned = useAppStore((state) => state.relationshipCardPinned)
  const close = useAppStore((state) => state.closeRelationshipCard)
  const setPinned = useAppStore((state) => state.setRelationshipCardPinned)
  const setSelectedEntityID = useAppStore((state) => state.setSelectedEntityId)
  const [position, setPosition] = useState(DEFAULT_POSITION)
  const cardRef = useRef<HTMLElement | null>(null)
  const dragStart = useRef<DragStart | null>(null)

  useEffect(() => {
    if (cardEntityID) setPosition(DEFAULT_POSITION)
  }, [cardEntityID])

  const entity = useMemo(
    () => snapshot?.graph.nodes.find((node) => node.id === cardEntityID),
    [cardEntityID, snapshot],
  )
  const entries = useMemo(
    () => cardEntityID && snapshot
      ? relationshipEntriesFor(snapshot.graph.nodes, snapshot.graph.edges, cardEntityID)
      : [],
    [cardEntityID, snapshot],
  )

  if (mode !== 'ownership' || !entity || entries.length === 0) return null

  const groups = groupEntries(entries)
  const onPointerDown = (event: PointerEvent<HTMLElement>) => {
    event.preventDefault()
    event.stopPropagation()
    event.currentTarget.setPointerCapture(event.pointerId)
    dragStart.current = {
      pointerX: event.clientX,
      pointerY: event.clientY,
      cardX: position.x,
      cardY: position.y,
    }
  }
  const onPointerMove = (event: PointerEvent<HTMLElement>) => {
    if (!dragStart.current) return
    event.preventDefault()
    event.stopPropagation()
    const surface = cardRef.current?.offsetParent as HTMLElement | null
    const card = cardRef.current
    const maxX = surface && card
      ? Math.max(12, surface.clientWidth - card.offsetWidth - 16)
      : Number.POSITIVE_INFINITY
    const maxY = surface && card
      ? Math.max(12, surface.clientHeight - card.offsetHeight - 16)
      : Number.POSITIVE_INFINITY
    setPosition({
      x: Math.min(maxX, Math.max(12, dragStart.current.cardX + event.clientX - dragStart.current.pointerX)),
      y: Math.min(maxY, Math.max(12, dragStart.current.cardY + event.clientY - dragStart.current.pointerY)),
    })
  }
  const onPointerEnd = () => {
    dragStart.current = null
  }

  return (
    <aside
      ref={cardRef}
      className={styles.card}
      data-testid="ownership-relationship-card"
      style={{ left: position.x, top: position.y }}
      aria-label={`Relationships for ${entity.name}`}
    >
      <header
        className={styles.header}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerEnd}
        onPointerCancel={onPointerEnd}
      >
        <span className={styles.dragHandle} aria-hidden="true">
          <GripVertical size={15} strokeWidth={2} />
        </span>
        <div className={styles.titleBlock}>
          <span className={styles.eyebrow}>Relationships</span>
          <strong className={styles.entityName}>{entity.name}</strong>
        </div>
        <button
          type="button"
          className={styles.iconButton}
          onClick={() => setPinned(!pinned)}
          onPointerDown={(event) => event.stopPropagation()}
          aria-label={pinned ? 'Unpin relationship card' : 'Pin relationship card'}
          title={pinned ? 'Unpin relationship card' : 'Pin relationship card'}
        >
          {pinned ? <Pin size={14} strokeWidth={2} /> : <PinOff size={14} strokeWidth={2} />}
        </button>
        <button
          type="button"
          className={styles.iconButton}
          onClick={close}
          onPointerDown={(event) => event.stopPropagation()}
          aria-label="Close relationship card"
        >
          <X size={15} strokeWidth={2} />
        </button>
      </header>
      <div className={styles.body}>
        {groups.map(([label, group]) => (
          <section key={label} className={styles.group}>
            <h2 className={styles.groupTitle}>{label} · {group.length}</h2>
            {group.map((entry) => (
              <button
                type="button"
                key={entry.id}
                className={styles.entry}
                onClick={() => {
                  setPinned(true)
                  setSelectedEntityID(entry.related.id)
                }}
                aria-label={`Select ${entry.related.kind} ${entry.related.name}`}
              >
                <span className={styles.kind}>{entry.related.kind}</span>
                <span className={styles.entryName}>{entry.related.name}</span>
                <span className={styles.entryNamespace}>{entry.related.namespace || 'cluster-scoped'}</span>
                <span className={styles.navigate}>↗</span>
              </button>
            ))}
          </section>
        ))}
      </div>
      <footer className={styles.footer}>
        {pinned ? 'Pinned while you inspect another entity' : 'Drag to compare · pin to keep open'}
      </footer>
    </aside>
  )
}

function groupEntries(entries: RelationshipEntry[]): Array<[string, RelationshipEntry[]]> {
  const groups = new Map<string, RelationshipEntry[]>()
  for (const entry of entries) {
    const label = entry.direction === 'outbound'
      ? 'Dependencies'
      : 'Dependents'
    const group = groups.get(label) ?? []
    group.push(entry)
    groups.set(label, group)
  }
  return [...groups.entries()]
}
