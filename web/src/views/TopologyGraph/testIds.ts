export function slugifyForTestId(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export function topologyEntityTestId(kind: string, name: string): string {
  return `topology-entity-${kind}-${slugifyForTestId(name)}`
}

export function topologyGroupTestId(kind: string, name: string): string {
  return `topology-group-${kind}-${slugifyForTestId(name)}`
}
