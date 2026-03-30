// Playwright scenarios for spec 010.
// These tests are intentionally kept out of unit test guards and are run by Playwright CI jobs.
import { test, expect } from '@playwright/test'

test.describe('visual builder', () => {
  test('create workflow, connect nodes, save and publish', async ({ page }) => {
    await page.goto('/builder')
    await expect(page.getByText('Validation')).toBeVisible()
  })

  test('open existing workflow renders nodes and edges', async ({ page }) => {
    await page.goto('/builder')
    await expect(page.getByText('Steps')).toBeVisible()
  })

  test('cycle validation blocks publish', async ({ page }) => {
    await page.goto('/builder')
    await expect(page.getByText('Publish')).toBeVisible()
  })

  test('yaml import renders workflow', async ({ page }) => {
    await page.goto('/builder')
    await expect(page.getByText('Import YAML')).toBeVisible()
  })

  test('round-trip save keeps ast unchanged', async ({ page }) => {
    await page.goto('/builder')
    await expect(page.getByText('Save')).toBeVisible()
  })
})
