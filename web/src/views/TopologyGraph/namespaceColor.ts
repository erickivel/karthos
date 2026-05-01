export const NAMESPACE_PALETTE = [
  '#22d3ee',
  '#a78bfa',
  '#f472b6',
  '#34d399',
  '#fbbf24',
  '#f87171',
  '#60a5fa',
  '#facc15',
] as const

export type NamespaceColor = (typeof NAMESPACE_PALETTE)[number]

// FNV-1a 32-bit hash; deterministic across runs.
function hash(input: string): number {
  let h = 0x811c9dc5
  for (let i = 0; i < input.length; i++) {
    h ^= input.charCodeAt(i)
    h = Math.imul(h, 0x01000193) >>> 0
  }
  return h
}

export function namespaceColor(namespace: string | undefined): NamespaceColor {
  const ns = namespace || ''
  const index = hash(ns) % NAMESPACE_PALETTE.length
  return NAMESPACE_PALETTE[index] as NamespaceColor
}
