import { expect, test, type Page } from '@playwright/test'
import { openTopology, setGranularity, setMode, waitForGraph } from './topology-helpers'

const MODES = ['ownership', 'infrastructure', 'traffic'] as const
const GRANULARITIES = ['overview', 'runtime', 'detailed'] as const

async function waitForSnapshotReady(page: Page, granularity: (typeof GRANULARITIES)[number]) {
  if (granularity === 'overview') return

  await expect
    .poll(async () => page.locator('[data-topology-dynamic="metrics"]').count())
    .toBeGreaterThan(0)
  await page.waitForTimeout(250)
}

test.describe('topology projection snapshots', () => {
  for (const mode of MODES) {
    for (const granularity of GRANULARITIES) {
      test(`${mode}-${granularity}`, async ({ page }) => {
        await openTopology(page, { polling: 'off' })
        await waitForGraph(page)
        await setMode(page, mode)
        await setGranularity(page, granularity)
        await waitForSnapshotReady(page, granularity)
        await expect(page.getByTestId('topology-graph')).toHaveScreenshot(
          `topology-${mode}-${granularity}.png`,
          {
            mask: [page.locator('[data-topology-dynamic="metrics"]')],
          },
        )
      })
    }
  }
})
