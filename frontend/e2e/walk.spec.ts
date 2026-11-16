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
    await expect(tours).toHaveCount(5);
    await expect(tours.nth(0)).toContainText(MAIN);
    await expect(tours.nth(0)).toContainText('1 problem');
    await expect(tours.nth(0)).toContainText('6 steps');

    const draft = page.getByRole('button', { name: /Reading an item/ });
    await expect(draft).toContainText('Unreviewed draft');
    await expect(draft).toContainText('2 problems');
    await expect(draft).toContainText('api, worker, billing');

    await expect(page.getByRole('button', { name: /broken\.tour/ })).toBeDisabled();
  });

  test('filters with / and opens with the keyboard', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.tour')).toHaveCount(5);

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
    await page.route('**/fixture.json', async (route) => {
      const response = await route.fetch();
      const fixture = (await response.json()) as { workspace: { root: string; name: string }; tours: unknown[] };
      // What the service reports when launched from Finder.
      await route.fulfill({ response, json: { ...fixture, workspace: { root: '', name: '' }, tours: [] } });
    });
    await page.goto('/');

    await expect(page.getByRole('heading', { level: 2, name: 'Open a folder to begin' })).toBeVisible();
    await page.getByRole('button', { name: 'Open a folder…' }).click();
    expect(await page.evaluate(() => window.gitActions)).toEqual(['choose-folder']);
  });
});
