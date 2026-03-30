// Playwright scenarios for spec 017.
import { test, expect } from '@playwright/test'

test.use({ viewport: { width: 375, height: 812 } })

test.describe('mobile responsive flow', () => {
  test('bottom tab bar navigation is visible and usable', async ({ page }) => {
    await page.goto('/inbox')
    await expect(page.locator('.bottom-tab-bar')).toBeVisible()
    await page.getByRole('link', { name: /cases/i }).click()
    await expect(page).toHaveURL(/\/cases/)
  })

  test('inbox uses card layout on mobile', async ({ page }) => {
    await page.goto('/inbox')
    await expect(page.locator('.mobile-list')).toBeVisible()
  })

  test('case list filters open in bottom sheet', async ({ page }) => {
    await page.goto('/cases')
    await page.getByRole('button', { name: /filters/i }).click()
    await expect(page.getByText('Filters')).toBeVisible()
  })
})
