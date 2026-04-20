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

  test('keeps traffic mode readable for a service with many pods', async ({ page }) => {
    await setMode(page, 'traffic')
    await setGranularity(page, 'runtime')

    await expect(
      entityCardsByAttributes(page, {
        kind: 'pod',
        namespace: 'oksg-demo',
        namePrefix: 'service-b-node-',
      }),
    ).toHaveCount(3)
    await expect(groupCardsByAttributes(page, { kind: 'node' })).toHaveCount(0)
    await expect(groupCardsByAttributes(page, { kind: 'namespace' })).toHaveCount(0)
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
})
