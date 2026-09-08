import { expect, test } from '@playwright/test'
import {
  clearKind,
  clearNamespace,
  entityCard,
  entityCardsByAttributes,
  expectInspectorSelection,
  groupCard,
  groupCardsByAttributes,
  openTopology,
  selectEntity,
  selectNamespace,
  setGranularity,
  setMode,
  waitForGraph,
} from './topology-helpers'

test.describe('topology projection structure', () => {
  test.beforeEach(async ({ page }) => {
    await openTopology(page)
    await waitForGraph(page)
  })

  test('groups cross-namespace pods under one node in infrastructure mode', async ({ page }) => {
    await setGranularity(page, 'runtime')
    await setMode(page, 'infrastructure')

    await expect(groupCard(page, 'node', 'oksg-test-control-plane')).toBeVisible()
    await expect(
      entityCardsByAttributes(page, {
        kind: 'pod',
        namespace: 'oksg-demo',
        namePrefix: 'gateway-go-',
      }),
    ).toHaveCount(1)
    await expect(entityCard(page, 'pod', 'ops-toolbox')).toBeVisible()
  })

  test('shows Kubernetes nodes as entity cards in infrastructure overview', async ({ page }) => {
    await setGranularity(page, 'overview')
    await setMode(page, 'infrastructure')

    await expect(entityCardsByAttributes(page, { kind: 'node' })).toHaveCount(2)
    await expect(groupCardsByAttributes(page, { kind: 'node' })).toHaveCount(0)
  })

  test('packs infrastructure runtime pods into compact node groups', async ({ page }) => {
    await setGranularity(page, 'runtime')
    await setMode(page, 'infrastructure')

    const group = groupCard(page, 'node', 'oksg-test-control-plane')
    await expect(group).toBeVisible()
    const box = await group.boundingBox()
    expect(box?.width).toBeLessThan(1_250)
  })

  test('groups same-namespace pods across nodes in ownership mode', async ({ page }) => {
    await selectNamespace(page, 'oksg-demo')
    await setGranularity(page, 'runtime')
    await setMode(page, 'ownership')

    await expect(groupCard(page, 'namespace', 'oksg-demo')).toBeVisible()
    await expect(
      entityCardsByAttributes(page, {
        kind: 'pod',
        namespace: 'oksg-demo',
        namePrefix: 'service-b-node-',
      }),
    ).toHaveCount(3)
  })

  test('Endpoint Flow shows ingress + services only, no workloads or pods', async ({ page }) => {
    await setMode(page, 'traffic')
    await setGranularity(page, 'overview')

    await expect(page.locator('[data-entity-kind="pod"]')).toHaveCount(0)
    await expect(page.locator('[data-entity-kind="workload"]')).toHaveCount(0)
    await expect(groupCardsByAttributes(page, { kind: 'node' })).toHaveCount(0)
    await expect(groupCardsByAttributes(page, { kind: 'namespace' })).toHaveCount(0)
  })

  test('Workload Flow adds workloads, still hides pods', async ({ page }) => {
    await setMode(page, 'traffic')
    await setGranularity(page, 'runtime')

    await expect(page.locator('[data-entity-kind="pod"]')).toHaveCount(0)
    await expect(
      page.locator('[data-entity-kind="workload"]').first(),
    ).toBeVisible()
    await expect(groupCardsByAttributes(page, { kind: 'node' })).toHaveCount(0)
    await expect(groupCardsByAttributes(page, { kind: 'namespace' })).toHaveCount(0)
  })

  test('Instance Flow renders pod replicas and no service/workload nodes', async ({ page }) => {
    await setMode(page, 'traffic')
    await setGranularity(page, 'detailed')

    await expect(page.locator('[data-entity-kind="service"]')).toHaveCount(0)
    await expect(page.locator('[data-entity-kind="workload"]')).toHaveCount(0)
    await expect(page.locator('[data-entity-kind="ingress"]')).toHaveCount(0)
    await expect(
      entityCardsByAttributes(page, {
        kind: 'pod',
        namespace: 'oksg-demo',
        namePrefix: 'service-b-node-',
      }),
    ).toHaveCount(3)
  })

  test('adds container detail in detailed mode without losing pod context', async ({ page }) => {
    await selectNamespace(page, 'oksg-demo')
    await setGranularity(page, 'runtime')
    await expect(entityCardsByAttributes(page, { kind: 'container' })).toHaveCount(0)

    await setGranularity(page, 'detailed')

    await expect(
      entityCardsByAttributes(page, {
        kind: 'container',
        namespace: 'oksg-demo',
        namePrefix: 'service-b-node',
      }),
    ).toHaveCount(3)
    await expect(
      entityCardsByAttributes(page, {
        kind: 'pod',
        namespace: 'oksg-demo',
        namePrefix: 'service-b-node-',
      }),
    ).toHaveCount(3)
  })

  test('degrades gracefully for sparse namespaces without workloads or services', async ({ page }) => {
    await selectNamespace(page, 'oksg-sparse')
    await setGranularity(page, 'runtime')
    await setMode(page, 'ownership')

    await waitForGraph(page)
    await expect(page.locator('[data-entity-kind]')).toHaveCount(1)
    await expect(entityCard(page, 'pod', 'ops-toolbox')).toBeVisible()
    await expect(page.locator('[data-entity-kind="workload"]')).toHaveCount(0)
    await expect(page.locator('[data-entity-kind="service"]')).toHaveCount(0)
  })

  test('keeps standalone pods visible as ungrouped fallbacks', async ({ page }) => {
    await clearNamespace(page)
    await setGranularity(page, 'runtime')
    await setMode(page, 'ownership')

    await expect(entityCard(page, 'pod', 'ops-toolbox')).toBeVisible()
  })

  test('preserves selection across mode switches and avoids duplicates across granularity switches', async ({
    page,
  }) => {
    await selectNamespace(page, 'oksg-sparse')
    await setGranularity(page, 'runtime')

    await selectEntity(page, 'pod', 'ops-toolbox')
    await expectInspectorSelection(page, 'ops-toolbox')

    await setMode(page, 'infrastructure')
    await expectInspectorSelection(page, 'ops-toolbox')

    await setGranularity(page, 'detailed')
    await setGranularity(page, 'runtime')

    await expect(entityCard(page, 'pod', 'ops-toolbox')).toHaveCount(1)
    await expect(
      entityCardsByAttributes(page, {
        kind: 'container',
        namespace: 'oksg-sparse',
        namePrefix: 'toolbox',
      }),
    ).toHaveCount(0)

    await clearKind(page)
  })

  test('renders selected node with streamlined inspector sections', async ({ page }) => {
    await selectNamespace(page, 'oksg-sparse')
    await setGranularity(page, 'runtime')

    await selectEntity(page, 'pod', 'ops-toolbox')
    await expectInspectorSelection(page, 'ops-toolbox')

    await expect(page.getByTestId('node-inspector')).toContainText('Technical Specs')
    await expect(page.getByTestId('node-inspector')).toContainText('Graph Metadata')
    await expect(page.getByTestId('node-inspector')).toContainText('Properties')
    await expect(page.getByTestId('node-inspector')).toContainText('top to bottom into this node')
    await expect(page.getByTestId('node-inspector')).toContainText('bottom to top away from this node')
  })

  test('keeps legend and node inspector mutually exclusive', async ({ page }) => {
    await page.getByTestId('topology-legend-toggle').click()
    await expect(page.getByTestId('topology-legend')).toBeVisible()
    await expect(page.getByTestId('node-inspector')).toHaveCount(0)

    await selectEntity(page, 'pod', 'ops-toolbox')

    await expectInspectorSelection(page, 'ops-toolbox')
    await expect(page.getByTestId('topology-legend')).toHaveCount(0)

    await page.getByTestId('topology-legend-toggle').click()

    await expect(page.getByTestId('topology-legend')).toBeVisible()
    await expect(page.getByTestId('node-inspector')).toHaveCount(0)
  })
})
