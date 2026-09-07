import { expect, test, type Page } from '@playwright/test'
import { openTopology, setGranularity, setMode, waitForGraph } from './topology-helpers'

const MODES = ['ownership', 'infrastructure', 'traffic'] as const
const GRANULARITIES = ['overview', 'runtime', 'detailed'] as const

async function waitForLayoutSettled(page: Page) {
  // Resource chips are optional: the current graph contract renders topology
  // independently from catalog-backed metric availability.
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
        await waitForLayoutSettled(page)
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
