import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';

// The frontend's share of the design's performance budgets, measured in the
// production bundle in WebKit. The Go share is in internal/perf.
//
// These run alone: a stopwatch shared with other pages measures the other pages.
test.describe.configure({ mode: 'serial' });

const FIRST_STEP_MS = 1000;
const NAVIGATION_MS = 50;
// A frame at 60 fps is 16.7 ms. One that takes more than two has visibly dropped.
const DROPPED_FRAME_MS = 34;

async function perfTour(page: Page): Promise<string> {
  await page.goto('/');
  return page.evaluate(async () => {
    const fixture = (await (await fetch('/fixture.json')).json()) as { tours: { path: string }[] };
    return fixture.tours.find((t) => t.path.endsWith('perf.tour'))!.path;
  });
}

test('the first step of a tour is on screen within a second of launch', async ({ page }) => {
  const tour = await perfTour(page);
  await page.addInitScript((path) => {
    window.docentLaunch = { tour: path, step: '' };
  }, tour);

  await page.reload();
  await page.waitForSelector('.row.band');
  const elapsed = await page.evaluate(() => performance.now());

  console.log(`first step rendered after ${Math.round(elapsed)} ms`);
  expect(elapsed).toBeLessThan(FIRST_STEP_MS);
  await expect(page.locator('.row.band .gutter')).toHaveText('15000');
});

test('moving to a step whose file is already loaded takes under 50 ms', async ({ page }) => {
  const tour = await perfTour(page);
  await page.addInitScript((path) => {
    window.docentLaunch = { tour: path, step: '' };
  }, tour);
  await page.reload();
  await page.waitForSelector('.row.band');
  // Let the next step finish warming, as it would while someone reads this one.
  await page.waitForTimeout(300);

  const elapsed = await page.evaluate(
    () =>
      new Promise<number>((resolve) => {
        const started = performance.now();
        const observer = new MutationObserver(() => {
          const band = document.querySelector('.row.band .gutter');
          if (band?.textContent === '40') {
            observer.disconnect();
            // Count the frame the change is painted in, not just the DOM update.
            requestAnimationFrame(() => resolve(performance.now() - started));
          }
        });
        observer.observe(document.body, { subtree: true, childList: true, characterData: true });
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j', bubbles: true }));
      }),
  );

  console.log(`step navigation took ${Math.round(elapsed)} ms`);
  expect(elapsed).toBeLessThan(NAVIGATION_MS);
});

test('a 20,000-line file scrolls without dropping frames', async ({ page }) => {
  const tour = await perfTour(page);
  await page.addInitScript((path) => {
    window.docentLaunch = { tour: path, step: '' };
  }, tour);
  await page.reload();
  await page.waitForSelector('.row.band .text span');

  // Only what is near the viewport is in the DOM, however long the file is.
  expect(await page.locator('.code .row').count()).toBeLessThan(300);

  const frames = await page.evaluate(
    () =>
      new Promise<number[]>((resolve) => {
        const pane = document.querySelector('.code') as HTMLElement;
        const deltas: number[] = [];
        let last = performance.now();
        let remaining = 180;

        requestAnimationFrame(function tick(now) {
          deltas.push(now - last);
          last = now;
          // A fast flick: 30 lines a frame.
          pane.scrollTop += 600;
          if (--remaining > 0) {
            requestAnimationFrame(tick);
          } else {
            resolve(deltas.slice(1));
          }
        });
      }),
  );

  const sorted = [...frames].sort((a, b) => a - b);
  const median = sorted[Math.floor(sorted.length / 2)];
  const dropped = frames.filter((ms) => ms > DROPPED_FRAME_MS).length;

  console.log(`scrolling: median frame ${median.toFixed(1)} ms, ${dropped} of ${frames.length} frames dropped`);
  expect(median).toBeLessThan(20);
  expect(dropped).toBeLessThanOrEqual(frames.length * 0.05);
});
