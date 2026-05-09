import { useEffect, type RefObject } from 'react'

type AnyRef = RefObject<HTMLElement | null>

export function useClickOutside(
  refs: AnyRef | AnyRef[],
  enabled: boolean,
  onOutside: () => void,
) {
  useEffect(() => {
    if (!enabled) return
    const refList = Array.isArray(refs) ? refs : [refs]
    const handler = (event: MouseEvent | TouchEvent) => {
      // Only react to primary (left) mouse button. Right/middle clicks open
      // context menus or trigger other browser behavior and shouldn't dismiss
      // popovers as a side-effect.
      if ('button' in event && event.button !== 0) return
      const target = event.target as Node | null
      if (!target) return
      for (const ref of refList) {
        const el = ref.current
        if (el && el.contains(target)) return
      }
      onOutside()
    }
    // Capture phase so this fires even when an inner handler (e.g. React
    // Flow's pane) calls stopPropagation on the bubbling pointer event.
    document.addEventListener('mousedown', handler, true)
    document.addEventListener('touchstart', handler, true)
    return () => {
      document.removeEventListener('mousedown', handler, true)
      document.removeEventListener('touchstart', handler, true)
    }
  }, [refs, enabled, onOutside])
}
