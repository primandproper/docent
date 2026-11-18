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
    await expect(tours).toHaveCount(6);
    await expect(tours.nth(0)).toContainText(MAIN);
    await expect(tours.nth(0)).toContainText('1 problem');
    await expect(tours.nth(0)).toContainText('6 steps');

    const draft = page.getByRole('button', { name: /Reading an item/ });
    await expect(draft).toContainText('Unreviewed draft');
    // One of them is its link to a step the other tour doesn't have.
    await expect(draft).toContainText('3 problems');
    await expect(draft).toContainText('api, worker, billing');

    await expect(page.getByRole('button', { name: /broken\.tour/ })).toBeDisabled();
  });

  test('filters with / and opens with the keyboard', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.tour')).toHaveCount(6);

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
    await expect(page.locator('.crumbs .badge')).toHaveText('+4 lines');
    await expect(page.locator('.narrative .notice')).toContainText('pattern matched on line 9, not line 5');
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

  test('history goes back and forward, across jumps, tours and the picker', async ({ page }) => {
    await openMain(page);

    // Stepping takes the history entry along; a jump adds one.
    await page.keyboard.press('j');
    await expect(currentStep(page)).toContainText('The write handler');
    await page.locator('.step').nth(4).click();
    await page.keyboard.press('j');
    const last = await currentStep(page).textContent();

    await page.keyboard.press('Meta+[');
    await expect(currentStep(page)).toContainText('The write handler');
    await page.keyboard.press('Meta+ArrowLeft');
    await expect(page.locator('.picker')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Back', exact: true })).toBeDisabled();

    await page.keyboard.press('Meta+]');
    await expect(currentStep(page)).toContainText('The write handler');
    await page.keyboard.press('Meta+ArrowRight');
    await expect(currentStep(page)).toHaveText(last ?? '');
    await expect(page.getByRole('button', { name: 'Forward', exact: true })).toBeDisabled();
  });

  test('⌘← in the search box moves the caret, not the history', async ({ page }) => {
    await openMain(page);
    await page.keyboard.press('/');
    await expect(page.getByRole('searchbox')).toBeFocused();

    await page.keyboard.press('Meta+ArrowLeft');
    await expect(page.locator('.picker')).toBeVisible();
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

test.describe('a tour across repos', () => {
  async function openDraft(page: Page): Promise<void> {
    await page.goto('/');
    await page.getByRole('button', { name: /Reading an item/ }).click();
    await expect(page.getByRole('heading', { level: 1, name: 'Reading an item' })).toBeVisible();
  }

  test('steps read from their own repos, however each was found', async ({ page }) => {
    await openDraft(page);

    await expect(page.locator('.crumbs .root')).toHaveText('api');
    await expect(page.locator('.row.band')).toContainText('func (h *Handler) GetItem');
    await expect(page.locator('.step').nth(0).locator('.repo')).toHaveText('api');

    await page.keyboard.press('j');
    await expect(page.locator('.crumbs .root')).toHaveText('worker');
    await expect(page.locator('.row.band')).toContainText('const MAX_ATTEMPTS = 5;');
  });

  test('a repo that is not checked out offers its clone command', async ({ page }) => {
    await openDraft(page);

    await page.locator('.step').nth(2).click();
    await expect(page.locator('.crumbs .badge')).toHaveText('Missing');
    await expect(page.locator('.narrative .notice')).toContainText('repo "billing" is not checked out on this machine');
    await expect(page.locator('.clone code')).toContainText('git clone git@github.com:acme/billing.git');

    await page.locator('.clone').getByRole('button', { name: 'Copy' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Copied the clone command.' })).toBeVisible();
    expect((await page.evaluate(() => window.copied))[0]).toMatch(
      /^git clone git@github\.com:acme\/billing\.git .*\/billing$/,
    );
  });

  test('the body file replaces the description, and links become cards', async ({ page }) => {
    await openDraft(page);

    const narrative = page.locator('.narrative');
    await expect(narrative.locator('table')).toContainText('Cache miss');
    await expect(narrative).not.toContainText('Reads go straight to the store on a cache miss.');

    const cards = narrative.locator('.links .link-card');
    await expect(cards).toHaveCount(2);
    await expect(cards.nth(0)).toContainText('grafana');
    await expect(cards.nth(0)).toContainText('Read latency p99');
    await expect(cards.nth(0)).toContainText('grafana.example.com');
    await expect(cards.nth(1)).toBeDisabled();
    await expect(cards.nth(1)).toContainText('No value for {{logs}}');
  });

  test('picking a variable changes where the links go', async ({ page }) => {
    await openDraft(page);

    const card = page.locator('.links .link-card').first();
    await card.click();
    await page.getByLabel('env').selectOption('prod');
    await expect(card).toHaveAttribute('title', /var-env=prod/);
    await card.click();

    expect(await page.evaluate(() => window.openedURLs)).toEqual([
      'https://grafana.example.com/d/abc123/items?var-env=staging',
      'https://grafana.example.com/d/abc123/items?var-env=prod',
    ]);
  });

  test('o opens the step in the editor, and only for steps with a file', async ({ page }) => {
    await openDraft(page);
    await expect(page.locator('.row.band')).toBeVisible();

    await page.keyboard.press('o');
    await expect(page.getByRole('status').filter({ hasText: 'Opened in Zed.' })).toBeVisible();

    await page.locator('.step').nth(3).click();
    await expect(page.locator('.narrative')).toContainText('Short tour, short path.');
    await page.keyboard.press('o');

    const opened = await page.evaluate(() => window.openedInEditor);
    expect(opened).toHaveLength(1);
    expect(opened[0]).toMatchObject({ index: 0, editorID: '' });
  });

  test('a change on disk reloads the tour and keeps the reader on their step', async ({ page }) => {
    await openDraft(page);

    await page.keyboard.press('j');
    await expect(currentStep(page)).toContainText('Retries in the worker');

    // Stand in for an agent inserting a step above this one and retitling it.
    await page.evaluate(() => {
      const fixture = window.docentFixture!;
      const path = Object.keys(fixture.views).find((p) => p.endsWith('reading-an-item.tour'))!;
      const view = fixture.views[path];
      const steps = view.tour!.steps!;
      steps.unshift({ ...steps[3], id: 'preface', title: 'A new first step' });
      view.steps!.unshift(view.steps![3]);
      fixture.steps[path].unshift({ ...fixture.steps[path][3] });
      fixture.steps[path].forEach((step, index) => (step.index = index));
      steps[2].title = 'Retries, retitled';
      window.docentChanged();
    });

    await expect(page.locator('.step')).toHaveCount(7);
    await expect(currentStep(page)).toContainText('Retries, retitled');
    await expect(page.locator('.row.band')).toContainText('const MAX_ATTEMPTS = 5;');
  });
});

test.describe('drift', () => {
  async function openDraft(page: Page): Promise<void> {
    await page.goto('/');
    await page.getByRole('button', { name: /Reading an item/ }).click();
    await expect(page.getByRole('heading', { level: 1, name: 'Reading an item' })).toBeVisible();
  }

  test('the banner says how far each repo is from what the tour was written against', async ({ page }) => {
    await openDraft(page);

    const banner = page.getByRole('list', { name: 'How current this tour is' });
    await expect(banner.getByRole('listitem')).toHaveCount(2);
    await expect(banner).toContainText(
      /worker: written against [0-9a-f]{7}, 1 commit \/ \d+ \w+ behind main · 1 moved · 1 changed · 1 missing/,
    );
    await expect(banner).toContainText("billing isn't checked out on this machine");
    // A repo with nothing to report isn't mentioned.
    await expect(banner).not.toContainText('api');
  });

  test('code is read at the ref, and says which commit', async ({ page }) => {
    await openDraft(page);

    await expect(page.locator('.crumbs .ref')).toHaveText(/^main @ [0-9a-f]{7}$/);

    await page.getByRole('button', { name: 'Copy permalink' }).click();
    expect((await page.evaluate(() => window.copied))[0]).toMatch(
      /^https:\/\/github\.com\/acme\/api\/blob\/[0-9a-f]{40}\/internal\/http\/items\.go#L15$/,
    );
  });

  test('an anchor that only shifted is followed exactly', async ({ page }) => {
    await openDraft(page);

    await page.locator('.step').nth(1).click();
    await expect(page.locator('.crumbs .badge')).toHaveText('+2 lines');
    await expect(page.locator('.row.band')).toContainText('const MAX_ATTEMPTS = 5;');
    await expect(page.locator('.narrative .notice')).toContainText(/moved \+2 lines since [0-9a-f]{7}/);
  });

  test('edited code offers the original beside the current', async ({ page }) => {
    await openDraft(page);

    await page.locator('.step').nth(4).click();
    await expect(page.locator('.crumbs .badge')).toHaveText('Code changed');
    await expect(page.locator('.compare')).toHaveCount(0);

    await page.getByRole('button', { name: 'Then / Now' }).click();
    const sides = page.locator('.compare .side');
    await expect(sides).toHaveCount(2);
    await expect(sides.nth(0).locator('.caption')).toHaveText(/Then, at [0-9a-f]{7}/);
    await expect(sides.nth(0)).not.toContainText('log.warn');
    // The original keeps its own line numbers.
    await expect(sides.nth(0).locator('.gutter').first()).toHaveText('7');
    await expect(sides.nth(1)).toContainText("log.warn('purge failed'");
  });

  test('removed code shows what it was and the commit that removed it', async ({ page }) => {
    await openDraft(page);

    await page.locator('.step').nth(5).click();
    await expect(page.locator('.crumbs .badge')).toHaveText('Missing');
    await expect(page.locator('.compare.alone .side')).toHaveCount(1);
    await expect(page.locator('.compare')).toContainText('function sleep(ms: number): Promise<void> {');
    await expect(page.locator('.narrative')).toContainText(
      /Removed in [0-9a-f]{7}: use timers\/promises for sleep, and log failed purges/,
    );
  });

  test('cloning a missing repo shows the command and waits for a yes', async ({ page }) => {
    await openDraft(page);

    const banner = page.getByRole('list', { name: 'How current this tour is' });
    await banner.getByRole('button', { name: 'Clone…' }).click();
    expect(await page.evaluate(() => window.gitActions)).toEqual([]);

    const confirm = banner.getByRole('group', { name: 'Confirm clone' });
    await expect(confirm.locator('code')).toContainText('git clone git@github.com:acme/billing.git');

    await confirm.getByRole('button', { name: 'Cancel' }).click();
    await expect(confirm).toHaveCount(0);
    expect(await page.evaluate(() => window.gitActions)).toEqual([]);

    await banner.getByRole('button', { name: 'Clone…' }).click();
    await banner.getByRole('button', { name: 'Run it' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Cloned billing.' })).toBeVisible();
    expect(await page.evaluate(() => window.gitActions)).toEqual(['clone billing']);
  });

  test('the working tree toggle reads from disk and hides the banner', async ({ page }) => {
    await openDraft(page);

    const toggle = page.getByRole('button', { name: 'Working tree' });
    await expect(toggle).toHaveAttribute('aria-pressed', 'false');

    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-pressed', 'true');
    await expect(page.getByRole('list', { name: 'How current this tour is' })).toHaveCount(0);
    expect(await page.evaluate(() => window.gitActions)).toEqual(['working-tree true']);
  });
});

test.describe('the system map', () => {
  async function openDraft(page: Page): Promise<void> {
    await page.goto('/');
    await page.getByRole('button', { name: /Reading an item/ }).click();
    await expect(page.getByRole('region', { name: 'System map' }).locator('svg')).toBeVisible();
  }

  function node(page: Page, id: string) {
    return page.locator(`.map g.node[id*="-flowchart-${id}-"]`);
  }

  test('highlights where the current step sits and dims where the reader has been', async ({ page }) => {
    await openDraft(page);

    await expect(node(page, 'api')).toHaveClass(/current/);
    await expect(node(page, 'store')).toHaveClass(/current/);
    await expect(page.locator('.map path[data-id="e1"]')).toHaveClass(/current/);
    await expect(node(page, 'worker')).toHaveClass(/upcoming/);
    await expect(node(page, 'client')).not.toHaveClass(/current|visited|upcoming/);

    await page.keyboard.press('j');
    await expect(node(page, 'worker')).toHaveClass(/current/);
    await expect(page.locator('.map path[data-id="e3"]')).toHaveClass(/current/);
    await expect(node(page, 'api')).toHaveClass(/visited/);
    await expect(node(page, 'api')).not.toHaveClass(/current/);
  });

  test('clicking a node jumps to the first step that focuses it', async ({ page }) => {
    await openDraft(page);

    await node(page, 'billing').click();
    await expect(currentStep(page)).toContainText('Where reads are metered');

    await node(page, 'worker').click();
    await expect(currentStep(page)).toContainText('Retries in the worker');
  });

  test('m hides and shows it', async ({ page }) => {
    await openDraft(page);

    await page.keyboard.press('m');
    await expect(page.getByRole('region', { name: 'System map' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Map' })).toHaveAttribute('aria-pressed', 'false');

    await page.keyboard.press('m');
    await expect(page.getByRole('region', { name: 'System map' }).locator('svg')).toBeVisible();
  });

  test('it can take the centre pane, and still follows the tour from there', async ({ page }) => {
    await openDraft(page);

    const map = page.getByRole('region', { name: 'System map' });
    const small = (await map.locator('svg').boundingBox())!;

    await page.keyboard.press('Shift+M');
    await expect(map).toHaveClass(/large/);
    await expect(page.locator('.pane')).toHaveCount(0);
    await expect(page.locator('.narrative')).toBeVisible();
    await expect(page.getByRole('region', { name: 'System map' })).toHaveCount(1);

    const large = (await map.locator('svg').boundingBox())!;
    expect(large.width).toBeGreaterThan(small.width * 1.5);

    // Still the tour's index, and still tracking the current step.
    await expect(node(page, 'api')).toHaveClass(/current/);
    await node(page, 'worker').click();
    await expect(currentStep(page)).toContainText('Retries in the worker');
    await expect(node(page, 'worker')).toHaveClass(/current/);

    // It stays large on a step with no code of its own.
    await page.locator('.step').nth(3).click();
    await expect(map).toHaveClass(/large/);

    await map.getByRole('button', { name: 'Smaller' }).click();
    await expect(map).not.toHaveClass(/large/);

    // Escape leaves the large map before it leaves the tour.
    await map.hover();
    await map.getByRole('button', { name: 'Larger' }).click();
    await page.keyboard.press('Escape');
    await expect(map).not.toHaveClass(/large/);
    await expect(page.getByRole('heading', { level: 1, name: 'Reading an item' })).toBeVisible();
  });

  function viewBox(target: Page) {
    return target.locator('.diagram svg').evaluate((svg) => svg.getAttribute('viewBox')!.split(' ').map(Number));
  }

  test('z zooms in on what the step is about, and follows the tour from there', async ({ page }) => {
    await openDraft(page);
    const map = page.getByRole('region', { name: 'System map' });
    const whole = await viewBox(page);

    await page.keyboard.press('z');
    await expect(map.getByRole('button', { name: 'Zoom' })).toHaveAttribute('aria-pressed', 'true');
    // The animation settles on a smaller part of the diagram, with the step's nodes in it.
    await expect.poll(async () => (await viewBox(page))[2]).toBeLessThan(whole[2]);
    await page.waitForTimeout(500);
    const first = await viewBox(page);
    const frame = (await map.locator('svg').boundingBox())!;
    const api = (await node(page, 'api').boundingBox())!;
    expect(api.x).toBeGreaterThanOrEqual(frame.x);
    expect(api.x + api.width).toBeLessThanOrEqual(frame.x + frame.width);

    // The next step is about another part of the system, and the zoom goes there.
    await page.keyboard.press('j');
    await expect(node(page, 'worker')).toHaveClass(/current/);
    await expect.poll(() => viewBox(page)).not.toEqual(first);
    await page.waitForTimeout(500);
    const worker = (await node(page, 'worker').boundingBox())!;
    expect(worker.x).toBeGreaterThanOrEqual(frame.x);
    expect(worker.x + worker.width).toBeLessThanOrEqual(frame.x + frame.width);

    // A step about one node closes right in on it.
    await node(page, 'billing').click();
    await expect.poll(async () => (await viewBox(page))[2]).toBeLessThan(whole[2] / 2);

    // And back out to all of it.
    await page.keyboard.press('z');
    await expect.poll(() => viewBox(page)).toEqual(whole);
    await expect(map.getByRole('button', { name: 'Zoom' })).toHaveAttribute('aria-pressed', 'false');
  });

  test('⌃Z pops it out into a window that still follows the tour', async ({ page, context }) => {
    await openDraft(page);
    const docked = page.getByRole('region', { name: 'System map' });

    const [popup] = await Promise.all([context.waitForEvent('page'), page.keyboard.press('Control+z')]);
    const popped = (id: string) => popup.locator(`g.node[id*="-flowchart-${id}-"]`);

    // The map moves: it is in its own window, and no longer in this one.
    await expect(popped('api')).toHaveClass(/current/);
    await expect(docked).toHaveCount(0);

    // Stepping in the main window moves the highlight in the map window.
    await page.keyboard.press('j');
    await expect(popped('worker')).toHaveClass(/current/);
    await expect(popped('api')).toHaveClass(/visited/);

    // z in the main window zooms the map where it now is.
    const whole = await viewBox(popup);
    await page.keyboard.press('z');
    await expect.poll(async () => (await viewBox(popup))[2]).toBeLessThan(whole[2]);
    await page.keyboard.press('z');

    // The map window can drive the tour too: a node, or the same keys.
    await popped('billing').click();
    await expect(currentStep(page)).toContainText('Where reads are metered');
    await popup.keyboard.press('k');
    await expect(currentStep(page)).toContainText('Retries in the worker');
    await expect(popped('worker')).toHaveClass(/current/);

    // Leaving the tour leaves the window with nothing to draw.
    await page.keyboard.press('Escape');
    await expect(popup.getByText('Open a tour with a system map')).toBeVisible();
    await page.getByRole('button', { name: /Reading an item/ }).click();
    await expect(popped('api')).toHaveClass(/current/);

    // Closing it puts the map back where it was.
    await Promise.all([popup.waitForEvent('close'), popup.keyboard.press('Escape').catch(() => undefined)]);
    await expect(docked.locator('svg')).toBeVisible();
  });

  test('a tour without a map has no map panel or button', async ({ page }) => {
    await openMain(page);

    await expect(page.getByRole('region', { name: 'System map' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Map' })).toHaveCount(0);
  });
});

test.describe('links within the app', () => {
  async function openWrapUp(page: Page): Promise<void> {
    await page.goto('/');
    await page.getByRole('button', { name: /Reading an item/ }).click();
    await page.locator('.step').nth(3).click();
    await expect(page.locator('.narrative')).toContainText('Short tour, short path.');
  }

  test('a docent:step link jumps to that step', async ({ page }) => {
    await openWrapUp(page);

    await page.getByRole('link', { name: 'back to the handler' }).click();
    await expect(currentStep(page)).toContainText('The read handler');
    await expect(page).toHaveURL('/');
  });

  test('a docent:tour link opens the other tour at a step', async ({ page }) => {
    await openWrapUp(page);

    await page.getByRole('link', { name: 'the write path' }).click();
    await expect(page.getByRole('heading', { level: 1, name: MAIN })).toBeVisible();
    // That tour is plain CodeTour and has no step ids, so it opens at the start.
    await expect(currentStep(page)).toContainText('Why this is worth a tour');
  });

  test('the back button returns from a linked tour to the step the link was on', async ({ page }) => {
    await openWrapUp(page);

    await page.getByRole('link', { name: 'the write path' }).click();
    await expect(page.getByRole('heading', { level: 1, name: MAIN })).toBeVisible();
    await page.keyboard.press('j');
    await page.keyboard.press('j');

    await page.getByRole('button', { name: 'Back', exact: true }).click();
    await expect(page.getByRole('heading', { level: 1, name: 'Reading an item' })).toBeVisible();
    await expect(page.locator('.narrative')).toContainText('Short tour, short path.');

    await page.getByRole('button', { name: 'Forward', exact: true }).click();
    await expect(page.getByRole('heading', { level: 1, name: MAIN })).toBeVisible();
    await expect(currentStep(page)).toContainText('The purge worker');
  });

  test('a link card can point inside the app too', async ({ page }) => {
    await openWrapUp(page);

    await page.locator('.links .link-card').click();
    await expect(currentStep(page)).toContainText('The read handler');
    expect(await page.evaluate(() => window.openedURLs)).toEqual([]);
  });

  test('a mermaid fence is drawn, and its docent click directive works', async ({ page }) => {
    await openWrapUp(page);

    const figure = page.locator('.narrative figure.diagram');
    await expect(figure.locator('svg')).toBeVisible();
    await expect(figure).toContainText('Cache miss');

    await figure.locator('g.node[id*="-flowchart-miss-"]').click();
    await expect(currentStep(page)).toContainText('The read handler');
  });
});

test.describe('the code pane', () => {
  async function openDraft(page: Page): Promise<void> {
    await page.goto('/');
    await page.getByRole('button', { name: /Reading an item/ }).click();
    await expect(page.locator('.row.band')).toContainText('func (h *Handler) GetItem');
  }

  test('extra anchors are tabs, each with its own code, repo and status', async ({ page }) => {
    await openDraft(page);

    const tabs = page.getByRole('tablist', { name: 'Code for this step' }).getByRole('tab');
    await expect(tabs).toHaveText(['items.go', 'writer', /purge/]);
    await expect(tabs.nth(0)).toHaveAttribute('aria-selected', 'true');

    await tabs.nth(1).click();
    await expect(page.locator('.row.band')).toContainText('func (h *Handler) UpdateItem');
    await expect(page.locator('.crumbs')).toContainText('line 26');

    // The third anchor is in another repo, where the code has moved since the stamp.
    await tabs.nth(2).click();
    await expect(page.locator('.crumbs .root')).toHaveText('worker');
    await expect(page.locator('.crumbs .badge')).toHaveText('+2 lines');
    await expect(tabs.nth(2).locator('.dot')).toBeVisible();

    await page.keyboard.press('o');
    await expect(page.getByRole('status').filter({ hasText: 'Opened in Zed.' })).toBeVisible();
    expect(await page.evaluate(() => window.openedInEditor)).toEqual([expect.objectContaining({ index: 0, extra: 1 })]);

    // Moving to another step goes back to that step's own code.
    await page.keyboard.press('j');
    await expect(page.getByRole('tablist')).toHaveCount(0);
    await expect(page.locator('.row.band')).toContainText('const MAX_ATTEMPTS = 5;');
  });

  test('Open in… picks an editor for this step, and offers a rule to keep', async ({ page }) => {
    await openDraft(page);

    await page.keyboard.press('Shift+O');
    const menu = page.getByRole('menu', { name: 'Open in…' });
    await expect(menu.getByRole('menuitem')).toHaveText(['Zed', 'VS Code']);

    await menu.getByRole('button', { name: 'Copy rule' }).nth(1).click();
    expect(await page.evaluate(() => window.copied)).toEqual(['{"match":"**/*.go","use":"code"}']);

    await menu.getByRole('menuitem', { name: 'VS Code' }).click();
    await expect(menu).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: 'Opened in VS Code.' })).toBeVisible();
    expect(await page.evaluate(() => window.openedInEditor)).toEqual([
      expect.objectContaining({ editorID: 'code', extra: -1 }),
    ]);
  });

  test("the file drawer browses the step's repo and comes back", async ({ page }) => {
    await openDraft(page);

    await page.getByRole('button', { name: 'Files' }).click();
    const drawer = page.getByRole('navigation', { name: 'Files' });
    await expect(drawer.getByRole('button', { name: /internal/ })).toBeVisible();

    await drawer.getByRole('button', { name: /internal/ }).click();
    await drawer.getByRole('button', { name: /http/ }).click();
    await drawer.getByRole('button', { name: 'items.go' }).click();

    await expect(page.locator('.crumbs')).toContainText(/internal\s*›\s*http\s*›\s*items\.go/);
    await expect(page.locator('.row.band')).toHaveCount(0);
    await expect(page.locator('.code')).toContainText('package http');

    await page.getByRole('button', { name: 'Back to the step' }).click();
    await expect(page.locator('.row.band')).toContainText('func (h *Handler) GetItem');
  });
});

test.describe('following a call', () => {
  /** Records the name each call transition carries, as it plays. */
  async function open(page: Page): Promise<() => Promise<string[]>> {
    await page.goto('/');
    await page.getByRole('button', { name: /Following a call/ }).click();
    await expect(page.locator('.row.band-start .gutter')).toHaveText('9');
    await page.evaluate(() => {
      const played: string[] = [];
      (window as unknown as { played: string[] }).played = played;
      new MutationObserver((records) => {
        for (const record of records) {
          for (const added of record.addedNodes) {
            if (added instanceof HTMLElement && added.classList.contains('dive-name')) {
              played.push(added.textContent ?? '');
            }
          }
        }
      }).observe(document.body, { subtree: true, childList: true });
    });
    return () => page.evaluate(() => (window as unknown as { played: string[] }).played);
  }

  test('a step that asks for it opens out of the call, and folds back into it', async ({ page }) => {
    const played = await open(page);

    await page.keyboard.press('j');
    await expect.poll(played).toEqual(['respond']);
    // It ends with the pane as it would have been anyway.
    await expect(page.locator('.dive-name, .dive-copy')).toHaveCount(0);
    await expect(page.locator('.content > .code')).toHaveCount(1);
    await expect(page.locator('.row.band-start .gutter')).toHaveText('19');

    await page.keyboard.press('k');
    await expect.poll(played).toEqual(['respond', 'respond']);
    await expect(page.locator('.dive-name, .dive-copy')).toHaveCount(0);
    await expect(page.locator('.row.band-start .gutter')).toHaveText('9');
  });

  test('a step that did not ask simply appears', async ({ page }) => {
    const played = await open(page);

    await page.locator('.step').nth(2).click();
    await expect(page.locator('.row.band .gutter')).toHaveText('15');
    await page.keyboard.press('k');
    await expect(page.locator('.row.band-start .gutter')).toHaveText('19');
    await page.waitForTimeout(200);
    expect(await played()).toEqual([]);
  });

  test('it only plays when the call is on screen', async ({ page }) => {
    const played = await open(page);

    await page.locator('.content > .code').evaluate((pane) => (pane.scrollTop = pane.scrollHeight));
    await page.setViewportSize({ width: 1440, height: 260 });
    await page.locator('.content > .code').evaluate((pane) => (pane.scrollTop = 0));
    await expect(page.locator('.row[data-line="16"]')).not.toBeInViewport();

    await page.keyboard.press('j');
    await expect(page.locator('.row.band-start .gutter')).toHaveText('19');
    await page.waitForTimeout(200);
    expect(await played()).toEqual([]);
  });

  test('a reader who asked for less motion gets none', async ({ page }) => {
    await page.emulateMedia({ reducedMotion: 'reduce' });
    const played = await open(page);

    await page.keyboard.press('j');
    await expect(page.locator('.row.band-start .gutter')).toHaveText('19');
    await page.waitForTimeout(200);
    expect(await played()).toEqual([]);
  });
});

test.describe('layout overrides', () => {
  test('narrative hides the code, and code hides the narrative', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: /Every kind of visual/ }).click();

    // `layout: code` on a step with an anchor: the code takes the window.
    await page.locator('.step').nth(4).click();
    await expect(page.locator('.pane')).toBeVisible();
    await expect(page.locator('.narrative')).toBeHidden();
    await page.getByRole('tab', { name: 'handler' }).click();
    await expect(page.locator('.row.band')).toContainText('type Handler struct {');

    // `layout: narrative`: no code pane at all.
    await page.locator('.step').nth(3).click();
    await expect(page.locator('.pane')).toHaveCount(0);
    await expect(page.locator('.narrative')).toBeVisible();
  });
});

test.describe('opening from outside', () => {
  test('a requested tour and step open straight away', async ({ page }) => {
    await page.goto('/');
    const path = await page.evaluate(async () => {
      const fixture = (await (await fetch('/fixture.json')).json()) as { tours: { path: string }[] };
      return fixture.tours.find((t) => t.path.endsWith('reading-an-item.tour'))!.path;
    });

    // Stand in for `docent open <tour> --step retries`.
    await page.addInitScript((tour) => {
      window.docentLaunch = { tour, step: 'retries' };
    }, path);
    await page.reload();

    await expect(page.getByRole('heading', { level: 1, name: 'Reading an item' })).toBeVisible();
    await expect(currentStep(page)).toContainText('Retries in the worker');
  });

  test('with no folder, the app asks for one', async ({ page }) => {
    // What the service reports when launched from Finder with nothing saved.
    await page.addInitScript(() => {
      window.docentFolders = [];
    });
    await page.goto('/');

    await expect(page.getByRole('heading', { level: 2, name: 'Add a folder to begin' })).toBeVisible();
    await page.getByRole('button', { name: 'Add a folder…' }).click();
    expect(await page.evaluate(() => window.gitActions)).toEqual(['add-folder']);
    await expect(page.locator('.tour')).toHaveCount(6);
  });
});

test.describe('several folders', () => {
  const HANDBOOK = 'On call for the cache';

  async function addHandbook(page: Page): Promise<void> {
    await page.goto('/');
    await page.getByRole('button', { name: 'Add a folder…' }).click();
    await expect(page.getByRole('region', { name: 'handbook' })).toBeVisible();
  }

  test('a folder from the command line is open for now, until it is kept', async ({ page }) => {
    await page.goto('/');

    const workspace = page.getByRole('region', { name: 'workspace' });
    await expect(workspace).toContainText('Open for now');
    await expect(workspace.locator('.tour')).toHaveCount(6);

    await page.getByRole('button', { name: 'Keep workspace' }).click();
    await expect(workspace).not.toContainText('Open for now');
    await expect(page.getByRole('button', { name: 'Keep workspace' })).toHaveCount(0);
  });

  test('an added folder is listed under its own name, after the first', async ({ page }) => {
    await addHandbook(page);

    const folders = page.locator('section.folder');
    await expect(folders).toHaveCount(2);
    await expect(folders.nth(0).getByRole('heading', { level: 2 })).toHaveText('workspace');
    await expect(folders.nth(1).getByRole('heading', { level: 2 })).toHaveText('handbook');
    await expect(folders.nth(1).locator('.tour')).toHaveCount(1);
    await expect(folders.nth(1)).toContainText(HANDBOOK);
    // Its link into the other folder's tour is found, so there is nothing to report.
    await expect(folders.nth(1)).toContainText('All anchors resolve');
    await expect(folders.nth(1)).not.toContainText('Open for now');
  });

  test('search and the arrow keys run across folders', async ({ page }) => {
    await addHandbook(page);

    await page.keyboard.press('/');
    await page.keyboard.type('on-call');
    await expect(page.locator('section.folder')).toHaveCount(1);
    await expect(page.locator('.tour')).toHaveCount(1);
    await page.keyboard.press('Enter');
    await expect(page.getByRole('heading', { level: 1, name: HANDBOOK })).toBeVisible();

    await page.keyboard.press('Escape');
    await page.keyboard.press('/');
    for (let i = 0; i < 6; i++) {
      await page.keyboard.press('ArrowDown');
    }
    await page.keyboard.press('Enter');
    await expect(page.getByRole('heading', { level: 1, name: HANDBOOK })).toBeVisible();
  });

  test('a link reaches a tour in another folder, and back returns to the step it was on', async ({ page }) => {
    await addHandbook(page);
    await page.getByRole('button', { name: new RegExp(HANDBOOK) }).click();
    await page.keyboard.press('j');
    await expect(currentStep(page)).toContainText('How purges work');

    await page.getByRole('link', { name: 'how a write invalidates the cache' }).click();
    await expect(page.getByRole('heading', { level: 1, name: MAIN })).toBeVisible();
    await page.keyboard.press('j');

    await page.keyboard.press('Meta+ArrowLeft');
    await expect(page.getByRole('heading', { level: 1, name: HANDBOOK })).toBeVisible();
    await expect(currentStep(page)).toContainText('How purges work');

    // Stepping to that step added nothing to the history, so the next back is the picker.
    await page.keyboard.press('Meta+ArrowLeft');
    await expect(page.locator('.picker')).toBeVisible();
  });

  test('without the other folder, the link says the tour is not here', async ({ page }) => {
    // The first recorded folder is the one that starts open: make that the handbook.
    await page.route('**/fixture.json', async (route) => {
      const response = await route.fetch();
      const fixture = (await response.json()) as { folders: { root: string; name: string }[] };
      const handbook = fixture.folders.filter((folder) => folder.name === 'handbook');
      await route.fulfill({ response, json: { ...fixture, folders: handbook } });
    });
    await page.goto('/');
    await expect(page.locator('section.folder')).toHaveCount(1);

    await page.getByRole('button', { name: new RegExp(HANDBOOK) }).click();
    await page.keyboard.press('j');
    await page.getByRole('link', { name: 'how a write invalidates the cache' }).click();
    await expect(page.getByRole('alert')).toContainText('There is no tour "cache-invalidation.tour"');
    await expect(page.getByRole('heading', { level: 1, name: HANDBOOK })).toBeVisible();
  });

  test('removing a folder takes its tours away, and closes one that was open', async ({ page }) => {
    await addHandbook(page);

    await page.getByRole('button', { name: 'Remove workspace' }).click();
    await expect(page.locator('section.folder')).toHaveCount(1);
    await expect(page.locator('.tour')).toHaveCount(1);

    await page.getByRole('button', { name: 'Remove handbook' }).click();
    await expect(page.getByRole('heading', { level: 2, name: 'Add a folder to begin' })).toBeVisible();
  });
});

test.describe('marking a tour reviewed', () => {
  function mark(page: Page) {
    return page.getByRole('button', { name: /Mark reviewed/ });
  }

  test('a plain CodeTour file has no review to give', async ({ page }) => {
    await openMain(page);
    await expect(mark(page)).toHaveCount(0);
  });

  test('is offered only once every step has stayed on screen', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: /Every kind of visual/ }).click();
    await expect(page.getByRole('heading', { level: 1, name: 'Every kind of visual' })).toBeVisible();

    await expect(mark(page)).toHaveAttribute('aria-disabled', 'true');
    await expect(mark(page)).toContainText('0/5 read');

    // Passing through steps reads none of them.
    for (let i = 0; i < 4; i++) {
      await page.keyboard.press('j');
    }
    await expect(currentStep(page)).toContainText('5');
    await expect(mark(page)).toContainText('0/5 read');

    // Asking early says what is missing, and asks for nothing. (Forced: the
    // button is aria-disabled, which Playwright won't click but a reader can.)
    await mark(page).click({ force: true });
    await expect(page.getByRole('status')).toContainText('Read every step first.');
    await expect(page.getByRole('form', { name: 'Mark reviewed' })).toHaveCount(0);

    // Staying on each one does.
    for (let read = 1; read < 5; read++) {
      await expect(mark(page)).toContainText(`${read}/5 read`);
      await page.keyboard.press('k');
    }
    await expect(mark(page)).toHaveAttribute('aria-disabled', 'false');
    await expect(mark(page)).not.toContainText('read');
    await expect(page.locator('.rail .number.read')).toHaveCount(5);
    expect(await page.evaluate(() => window.reviewed)).toEqual([]);

    // Even then it asks before it writes, offering git's name.
    await mark(page).click();
    const form = page.getByRole('form', { name: 'Mark reviewed' });
    await expect(form.getByRole('textbox')).toHaveValue('Ada Reader');
    await form.getByRole('button', { name: 'Cancel' }).click();
    await expect(form).toHaveCount(0);
    expect(await page.evaluate(() => window.reviewed)).toEqual([]);

    await mark(page).click();
    await form.getByRole('textbox').fill('Grace Walker');
    await form.getByRole('button', { name: 'Sign and save' }).click();

    await expect(page.locator('.bar')).toContainText('Reviewed by Grace Walker');
    await expect(form).toHaveCount(0);
    const reviewed = await page.evaluate(() => window.reviewed);
    expect(reviewed).toHaveLength(1);
    expect(reviewed[0]).toMatch(/^Grace Walker: .*visuals\.tour$/);

    // The file changed, so vouching for it again means reading it again.
    await expect(mark(page)).toHaveAttribute('aria-disabled', 'true');
  });

  test('says what else is in the way, however much was read', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: /Reading an item/ }).click();
    await expect(page.locator('.bar')).toContainText('Unreviewed draft');

    await expect(mark(page)).toHaveAttribute('aria-disabled', 'true');
    await mark(page).click({ force: true });
    await expect(page.getByRole('status')).toContainText("The billing repo isn't checked out here.");
    await expect(page.getByRole('form', { name: 'Mark reviewed' })).toHaveCount(0);
  });
});
