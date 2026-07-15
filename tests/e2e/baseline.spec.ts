import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

test.describe('Docs_Hub UI/UX Baseline Suite', () => {

  test('Homepage baseline snapshot & accessibility check', async ({ page }) => {
    await page.goto('/');
    await expect(page).toHaveTitle(/Docs_Hub|Docs Hub/);

    // Take visual snapshot artifact
    await expect(page).toHaveScreenshot('homepage-baseline.png', { fullPage: true });

    // Axe accessibility audit
    const accessibilityScanResults = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
      .analyze();

    console.log(`Homepage accessibility violations: ${accessibilityScanResults.violations.length}`);
    expect(accessibilityScanResults.violations.length).toBeLessThanOrEqual(10); // Baseline threshold
  });

  test('Article reader baseline snapshot', async ({ page }) => {
    await page.goto('/a/enterprise-architecture-guidelines');
    await expect(page.locator('h1')).toContainText('Руководство по архитектуре Docs_Hub');

    await expect(page).toHaveScreenshot('reader-baseline.png', { fullPage: true });
  });

  test('Search responsiveness test', async ({ page }) => {
    await page.goto('/');
    const searchInput = page.locator('input[type="search"], input[name="q"]');
    if (await searchInput.count() > 0) {
      await searchInput.fill('архитектура');
      await searchInput.press('Enter');
    }
  });

});
