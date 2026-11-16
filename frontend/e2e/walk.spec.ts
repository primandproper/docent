import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';

const MAIN = 'Cache invalidation, end to end';

async function openMain(page: Page): Promise<void> {
  await page.goto('/');
  await page.getByRole('button', { name: new RegExp(MAIN) }).click();
  await expect(page.getByRole('heading', { level: 1, name: MAIN })).toBeVisible();
}

function currentStep(page: Page) {
  return page.locator('[aria-current="step"]');
}

test.describe('tour picker', () => {
  test('lists tours with their health, primary first', async ({ page }) => {
    await page.goto('/');

    const tours = page.locator('.tour');
    await expect(tours).toHaveCount(3);
    await expect(tours.nth(0)).toContainText(MAIN);
    await expect(tours.nth(0)).toContainText('1 problem');
    await expect(tours.nth(0)).toContainText('6 steps');

    const draft = page.getByRole('button', { name: /Reading an item/ });
    await expect(draft).toContainText('Unreviewed draft');
    await expect(draft).toContainText('All anchors resolve');

    await expect(page.getByRole('button', { name: /broken\.tour/ })).toBeDisabled();
  });

  test('filters with / and opens with the keyboard', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.tour')).toHaveCount(3);

    await page.keyboard.press('/');
    await expect(page.getByRole('searchbox')).toBeFocused();
    await page.keyboard.type('read-path');
    await expect(page.locator('.tour')).toHaveCount(1);

    await page.keyboard.press('Enter');
    await expect(page.getByRole('heading', { level: 1, name: 'Reading an item' })).toBeVisible();
    await expect(page.locator('.bar')).toContainText('Unreviewed draft');
  });
});

test.describe('walking a tour', () => {
  test('a content-only step gives the narrative the full width and sanitizes it', async ({ page }) => {
    const dialogs: string[] = [];
    page.on('dialog', (dialog) => {
      dialogs.push(dialog.message());
      void dialog.dismiss();
    });

    await openMain(page);

    await expect(page.locator('.pane')).toHaveCount(0);
    await expect(page.locator('.narrative')).toContainText('has to reach three systems');
    await expect(page.locator('.narrative pre')).toContainText('curl -X PUT');
    // Raw HTML in a tour is shown as text, never run.
    await expect(page.locator('.narrative')).toContainText("<script>alert('tours cannot run code')</script>");
    await expect(page.locator('.narrative script')).toHaveCount(0);
    expect(dialogs).toEqual([]);
  });

  test('j and k move between steps, and the code pane follows', async ({ page }) => {
    await openMain(page);

    await page.keyboard.press('j');
    await expect(currentStep(page)).toContainText('The write handler');
    await expect(page.locator('.crumbs')).toContainText('items.go');
    await expect(page.locator('.crumbs')).toContainText('lines 26–46');

    const band = page.locator('.row.band');
    await expect(band.first()).toContainText('func (h *Handler) UpdateItem');
    await expect(band).toHaveCount(21);
    await expect(band.first()).toBeInViewport();

    // Shiki colors arrive after the plain text.
    await expect(page.locator('.row.band .text span').first()).toBeVisible();

    await page.keyboard.press('ArrowDown');
    await expect(currentStep(page)).toContainText('The purge worker');
    await page.keyboard.press('k');
    await expect(currentStep(page)).toContainText('The write handler');
  });

  test('drifted and missing anchors say so', async ({ page }) => {
    await openMain(page);

    await page.locator('.step').nth(2).click();
    await expect(page.locator('.crumbs .badge')).toHaveText('+2 lines');
    await expect(page.locator('.narrative .notice')).toContainText('pattern matched on line 7, not line 5');
    await expect(page.locator('.row.band')).toContainText('export async function purge');

    await page.locator('.step').nth(3).click();
    await expect(page.locator('.crumbs .badge')).toHaveText('Missing');
    await expect(page.locator('.placeholder')).toContainText('"worker/src/cache.ts" does not exist');
    await expect(page.locator('.step').nth(3).locator('.dot.red')).toBeVisible();
    await expect(page.locator('.step').nth(2).locator('.dot.amber')).toBeVisible();
  });

  test('a directory step shows its listing and README', async ({ page }) => {
    await openMain(page);

    await page.locator('.step').nth(4).click();
    await expect(page.locator('.entries li')).toHaveText(['README.md', 'cache-runbook.md']);
    await expect(page.locator('.readme')).toContainText('Runbooks and diagrams');
  });

  test('links open in the system browser and never navigate the app', async ({ page }) => {
    await openMain(page);

    await page.keyboard.press('j');
    await page.getByRole('link', { name: 'cache runbook' }).click();
    await page.locator('.step').nth(5).click();
    await page.locator('.link-card').click();

    expect(await page.evaluate(() => window.openedURLs)).toEqual([
      'https://example.com/runbooks/cache',
      'https://grafana.example.com/d/abc123/items',
    ]);
    await expect(page).toHaveURL('/');
  });

  test('g goes to a step by number', async ({ page }) => {
    await openMain(page);

    await page.keyboard.press('g');
    const input = page.getByRole('dialog', { name: 'Go to step' }).getByRole('textbox');
    await expect(input).toBeFocused();

    await input.fill('9');
    await expect(input).toHaveAttribute('aria-invalid', 'true');
    await input.fill('5');
    await page.keyboard.press('Enter');

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(currentStep(page)).toContainText('The docs folder');
  });

  test('history goes back and forward, across tours and the picker', async ({ page }) => {
    await openMain(page);

    await page.keyboard.press('j');
    await page.keyboard.press('j');
    await expect(currentStep(page)).toContainText('The purge worker');

    await page.keyboard.press('Meta+[');
    await expect(currentStep(page)).toContainText('The write handler');
    await page.keyboard.press('Meta+[');
    await page.keyboard.press('Meta+[');
    await expect(page.locator('.picker')).toBeVisible();

    await page.keyboard.press('Meta+]');
    await page.keyboard.press('Meta+]');
    await expect(currentStep(page)).toContainText('The write handler');
  });

  test('the last step offers the next tour, and Escape returns to the picker', async ({ page }) => {
    await openMain(page);

    await page.locator('.step').nth(5).click();
    await page.getByRole('button', { name: 'Next tour: Reading an item' }).click();
    await expect(page.getByRole('heading', { level: 1, name: 'Reading an item' })).toBeVisible();
    await expect(page.locator('.row.band')).toContainText('func (h *Handler) GetItem');

    await page.keyboard.press('Escape');
    await expect(page.locator('.picker')).toBeVisible();
  });
});
