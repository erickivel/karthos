import { expect, type Locator, type Page } from '@playwright/test'

type ProjectionMode = 'ownership' | 'infrastructure' | 'traffic'
type Granularity = 'overview' | 'runtime' | 'detailed'

const MODE_LABEL: Record<ProjectionMode, string> = {
  ownership: 'Ownership',
  infrastructure: 'Infra',
  traffic: 'Traffic',
}

const OWNERSHIP_GRANULARITY_LABEL: Record<Granularity, string> = {
  overview: 'Workloads',
  runtime: 'Pods',
  detailed: 'Resources',
}

const INFRASTRUCTURE_GRANULARITY_LABEL: Record<Granularity, string> = {
  overview: 'Nodes',
  runtime: 'Pods',
  detailed: 'Containers',
}

const TRAFFIC_GRANULARITY_LABEL: Record<Granularity, string> = {
  overview: 'Endpoint',
  runtime: 'Workload',
  detailed: 'Instance',
}

function granularityLabel(mode: ProjectionMode, granularity: Granularity): string {
  if (mode === 'traffic') return TRAFFIC_GRANULARITY_LABEL[granularity]
  if (mode === 'infrastructure') return INFRASTRUCTURE_GRANULARITY_LABEL[granularity]
  return OWNERSHIP_GRANULARITY_LABEL[granularity]
}

function slugify(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export function entityTestId(kind: string, name: string) {
  return `topology-entity-${kind}-${slugify(name)}`
}

export function groupTestId(kind: string, name: string) {
  return `topology-group-${kind}-${slugify(name)}`
}

export async function openTopology(
  page: Page,
  options?: {
    polling?: 'off'
  },
) {
  const params = new URLSearchParams({ granularity: 'runtime' })
  if (options?.polling) params.set('polling', options.polling)

  await page.goto(`/topology?${params.toString()}`)
  await expect(page.getByTestId('topology-mode-group')).toBeVisible()
  await expect(page.getByTestId('topology-granularity-group')).toBeVisible()
  await waitForGraph(page)
}

export async function waitForGraph(page: Page) {
  await expect
    .poll(async () => page.locator('[data-entity-kind]').count(), {
      message: 'expected the topology graph to render at least one entity node',
    })
    .toBeGreaterThan(0)
}

export async function setMode(page: Page, mode: ProjectionMode) {
  await page.getByTestId(`mode-${mode}`).click()
  await expect(page.getByTestId(`mode-${mode}`)).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByTestId('topology-mode-group')).toContainText(MODE_LABEL[mode])
}

export async function setGranularity(page: Page, granularity: Granularity) {
  await page.getByTestId(`granularity-${granularity}`).click()
  await expect(page.getByTestId(`granularity-${granularity}`)).toHaveAttribute(
    'aria-pressed',
    'true',
  )
  // Each projection maps the stable URL granularity values to its own
  // user-facing vocabulary, so assert the active mode's label.
  const activeMode = await page.locator('[aria-pressed="true"][data-testid^="mode-"]').first().getAttribute('data-testid')
  const mode = (activeMode?.replace('mode-', '') ?? 'ownership') as ProjectionMode
  await expect(page.getByTestId('topology-granularity-group')).toContainText(
    granularityLabel(mode, granularity),
  )
}

export async function selectKind(page: Page, kind: string) {
  await page.getByTestId('filter-kind').selectOption(kind)
}

export async function selectNamespace(page: Page, namespace: string) {
  await page.getByTestId('filter-namespace').selectOption(namespace)
}

export async function clearNamespace(page: Page) {
  await page.getByTestId('filter-namespace').selectOption('')
}

export async function clearKind(page: Page) {
  await page.getByTestId('filter-kind').selectOption('')
}

export function entityCard(page: Page, kind: string, name: string): Locator {
  return page.getByTestId(entityTestId(kind, name))
}

export async function selectEntity(page: Page, kind: string, name: string) {
  await entityCard(page, kind, name).dispatchEvent('click')
}

export function entityCardsByAttributes(
  page: Page,
  options: {
    kind?: string
    namespace?: string
    namePrefix?: string
  },
): Locator {
  const parts = ['[data-entity-kind]']
  if (options.kind) parts.push(`[data-entity-kind="${options.kind}"]`)
  if (options.namespace) parts.push(`[data-entity-namespace="${options.namespace}"]`)
  if (options.namePrefix) parts.push(`[data-entity-name^="${options.namePrefix}"]`)
  return page.locator(parts.join(''))
}

export function groupCardsByAttributes(
  page: Page,
  options: {
    kind?: string
    namePrefix?: string
  },
): Locator {
  const parts = ['[data-group-kind]']
  if (options.kind) parts.push(`[data-group-kind="${options.kind}"]`)
  if (options.namePrefix) parts.push(`[data-group-name^="${options.namePrefix}"]`)
  return page.locator(parts.join(''))
}

export function groupCard(page: Page, kind: string, name: string): Locator {
  return page.getByTestId(groupTestId(kind, name))
}

export async function expectInspectorSelection(page: Page, entityName: string) {
  await expect(page.getByTestId('node-inspector')).toBeVisible()
  await expect(page.getByTestId('node-inspector-title')).toHaveText(entityName)
}
