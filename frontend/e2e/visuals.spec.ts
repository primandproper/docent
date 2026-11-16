import { expect, test } from '@playwright/test';
import type { Locator, Page } from '@playwright/test';

// Animation is switched off so every frame is the same picture each run.
test.beforeEach(async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce', colorScheme: 'light' });
  await page.goto('/');
  await page.getByRole('button', { name: /Every kind of visual/ }).click();
  await expect(page.getByRole('heading', { level: 1, name: 'Every kind of visual' })).toBeVisible();
});

function visual(page: Page, kind: string): Locator {
  return page.locator(`[data-visual="${kind}"]`);
}

// Snapshots are of the drawing only. A little slack absorbs font rendering
// differences between macOS versions.
const SNAPSHOT = { maxDiffPixelRatio: 0.01 };

test('a sequence plays its messages in order', async ({ page }) => {
  const seq = visual(page, 'sequence');

  await expect(seq.locator('.title')).toHaveText('Invalidating one item');
  await expect(seq.locator('.caption')).toContainText('client → api: PUT /items/42');
  await expect(seq.locator('.count')).toHaveText('1 / 4');
  await expect(seq.locator('.message.shown')).toHaveCount(1);
  await expect(seq.getByRole('button', { name: 'Previous frame' })).toBeDisabled();

  await seq.getByRole('button', { name: 'Next frame' }).click();
  await expect(seq.locator('.caption')).toContainText('api → queue: enqueue invalidate(42)');
  await expect(seq.locator('.message.current')).toHaveClass(/async/);
  await expect(seq.locator('.message.shown')).toHaveCount(2);

  await seq.getByRole('slider', { name: 'Scrub' }).fill('3');
  await expect(seq.locator('.count')).toHaveText('4 / 4');
  await expect(seq.getByRole('button', { name: 'Next frame' })).toBeDisabled();
  await expect(seq.locator('svg')).toHaveScreenshot('sequence.png', SNAPSHOT);
});

test('play runs to the end and stops, and starts over from there', async ({ page }) => {
  const seq = visual(page, 'sequence');

  await seq.getByRole('slider', { name: 'Scrub' }).fill('2');
  await seq.getByRole('button', { name: 'Play' }).click();
  await expect(seq.getByRole('button', { name: 'Pause' })).toBeVisible();
  await expect(seq.locator('.count')).toHaveText('4 / 4', { timeout: 4000 });
  await expect(seq.getByRole('button', { name: 'Play' })).toBeVisible({ timeout: 4000 });

  await seq.getByRole('button', { name: 'Play' }).click();
  await expect(seq.locator('.count')).toHaveText('1 / 4');
  await seq.getByRole('button', { name: 'Pause' }).click();
});

test('a frame can send the reader to the step it belongs to', async ({ page }) => {
  const seq = visual(page, 'sequence');

  await expect(seq.getByRole('button', { name: /Go to step/ })).toHaveCount(0);
  await seq.getByRole('button', { name: 'Next frame' }).click();
  await seq.getByRole('button', { name: 'Go to step 2' }).click();
  await expect(page.locator('[aria-current="step"]')).toContainText('A flow');
});

test("a flow, from the step's visual field, moves a packet along its path", async ({ page }) => {
  await page.locator('.step').nth(1).click();
  const flow = visual(page, 'flow');

  await expect(flow.locator('[data-node]')).toHaveCount(5);
  await expect(flow.locator('[data-node="cache"]')).toContainText('Edge cache');
  await expect(flow.locator('.caption')).toContainText('PUT /items/42: client → api');
  await expect(flow.locator('[data-edge="client->api"]')).toHaveClass(/hot/);
  await expect(flow.locator('[data-node="api"]')).toHaveClass(/hot/);
  await expect(flow.locator('.packet')).toHaveCount(1);

  await flow.getByRole('slider', { name: 'Scrub' }).fill('3');
  await expect(flow.locator('[data-edge="worker->cache"]')).toHaveClass(/hot/);
  await expect(flow.locator('[data-node="cache"]')).toHaveClass(/hot/);
  await expect(flow.locator('svg')).toHaveScreenshot('flow.png', SNAPSHOT);
});

test('a state machine starts at rest and follows its run', async ({ page }) => {
  await page.locator('.step').nth(2).click();
  const machine = visual(page, 'state');

  await expect(machine.locator('.caption')).toContainText('Starts in queued');
  await expect(machine.locator('[data-node="queued"]')).toHaveClass(/hot/);
  await expect(machine.locator('.count')).toHaveText('1 / 5');

  await machine.getByRole('button', { name: 'Next frame' }).click();
  await expect(machine.locator('.caption')).toContainText('start: queued → running');
  await expect(machine.locator('[data-node="running"]')).toHaveClass(/hot/);
  await expect(machine.locator('[data-node="queued"]')).not.toHaveClass(/hot/);

  await machine.getByRole('slider', { name: 'Scrub' }).fill('4');
  await expect(machine.locator('.caption')).toContainText('succeed: running → done');
  await expect(machine.locator('svg')).toHaveScreenshot('state.png', SNAPSHOT);
});

test('a timeline reveals its events in time order', async ({ page }) => {
  await page.locator('.step').nth(3).click();
  const timeline = visual(page, 'timeline');

  await expect(timeline.locator('.caption')).toContainText('0 ms: attempt 1');
  await expect(timeline.locator('.event.shown')).toHaveCount(1);

  await timeline.getByRole('slider', { name: 'Scrub' }).fill('3');
  await expect(timeline.locator('.caption')).toContainText('640 ms: purged');
  await expect(timeline.locator('.event.shown')).toHaveCount(4);
  await expect(timeline.locator('svg')).toHaveScreenshot('timeline.png', SNAPSHOT);
});

test('a docent-links fence becomes cards, and a broken fence says so without breaking the step', async ({ page }) => {
  await page.locator('.step').nth(3).click();

  const cards = page.locator('.narrative .embed .link-card');
  await expect(cards).toHaveCount(2);
  await expect(cards.nth(0)).toContainText('Purge latency');
  await expect(cards.nth(0)).toContainText('grafana.example.com');
  await expect(cards.nth(1)).toBeDisabled();

  await cards.nth(0).click();
  expect(await page.evaluate(() => window.openedURLs)).toEqual(['https://grafana.example.com/d/purge?var-env=staging']);

  await expect(page.locator('.narrative .broken')).toContainText("This visual can't be shown");
  await expect(page.locator('[data-visual="timeline"]')).toBeVisible();
});
