import { expect, test } from '@playwright/test';
import type { Locator, Page } from '@playwright/test';

const TOUR = 'Flags that steer the API';

// The pulse on a loading card is switched off so a screenshot is the same picture each run.
test.beforeEach(async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce', colorScheme: 'light' });
});

async function openFlags(page: Page): Promise<void> {
  await page.goto('/');
  await page.getByRole('button', { name: new RegExp(TOUR) }).click();
  await expect(page.getByRole('heading', { level: 1, name: TOUR })).toBeVisible();
}

function flagCard(page: Page, title: string): Locator {
  return page.locator('.flag-card').filter({ has: page.locator('.title', { hasText: title }) });
}

test.describe('flag cards', () => {
  test('a flag says how it decides, in the order it is evaluated', async ({ page }) => {
    await openFlags(page);

    const card = flagCard(page, 'New checkout flow');
    await expect(card).toHaveAttribute('data-state', 'on');
    await expect(card.locator('.type')).toHaveText('boolean');
    await expect(card.locator('.key')).toHaveText('new-checkout-flow');

    // On is not the whole story: most people still get the old path.
    const line = card.locator('.line');
    await expect(line.locator('.env')).toHaveText('Production');
    await expect(line.locator('.state')).toHaveText('On');
    await expect(line).toContainText('1 prerequisite, 3 named targets, 2 rules, then');
    await expect(line.locator('.chip')).toHaveText('v1');

    // Five things are decided before the default, so the list starts folded.
    const decides = card.locator('details.decides');
    await expect(decides).not.toHaveAttribute('open');
    await decides.locator('> summary').click();

    const steps = decides.locator('.steps > li');
    await expect(steps).toHaveCount(5);
    await expect(steps.nth(0)).toContainText('Only if flag pricing-service-v2 serves its variation 1');
    await expect(steps.nth(1)).toContainText('3 accounts named one by one');
    await expect(steps.nth(2)).toContainText('Beta customers');
    await expect(steps.nth(2)).toContainText('if user is in segment beta-testers or internal-staff');
    await expect(steps.nth(2).locator('.segment-about').nth(0)).toContainText('Beta testers 12 listed by name');
    await expect(steps.nth(2).locator('.segment-about').nth(1)).toContainText('user email ends with @acme.example.com');
    await expect(steps.nth(3)).toContainText('and account plan is not free');
    await expect(steps.nth(3).locator('.share')).toHaveText(['25% v2', '75% v1']);
    await expect(steps.nth(4)).toContainText('Everyone else');
    await expect(decides.locator('.off')).toContainText('If switched off: v1');

    // An ordinary card on the same step is still an ordinary card.
    await expect(page.locator('.links .link-card')).toHaveText(/Flag conventions/);
  });

  test('fourteen environments cost one line until they are asked for', async ({ page }) => {
    await openFlags(page);

    const others = flagCard(page, 'New checkout flow').locator('.others');
    await expect(others.locator('.other')).toHaveText([/^Staging/]);
    await expect(others.locator('.other').first()).toHaveClass(/on/);

    await others.getByRole('button', { name: '+12 more (4 on)' }).click();
    await expect(others.locator('.other')).toHaveCount(13);

    await others.getByRole('button', { name: 'fewer' }).click();
    await expect(others.locator('.other')).toHaveCount(1);
  });

  test('the card opens the flag in LaunchDarkly, in the browser', async ({ page }) => {
    await openFlags(page);

    await flagCard(page, 'New checkout flow').getByRole('button', { name: 'LaunchDarkly ↗' }).click();

    expect(await page.evaluate(() => window.openedURLs)).toEqual([
      'https://app.launchdarkly.com/default/production/features/new-checkout-flow',
    ]);
  });

  test('the status follows the environment the reader picks', async ({ page }) => {
    await openFlags(page);

    const line = flagCard(page, 'New checkout flow').locator('.line');
    await expect(line.locator('.env')).toHaveText('Production');

    await page.getByLabel('env').selectOption('staging');
    await expect(line.locator('.env')).toHaveText('Staging');
    await expect(line).toContainText('everyone gets');
    await expect(line.locator('.chip')).toHaveText('v2');
    // Nothing is decided before the default there, so there is no list to unfold.
    await expect(flagCard(page, 'New checkout flow').locator('details.decides')).toHaveCount(0);
  });

  test('a JSON flag shows each value as the result of the rule that serves it', async ({ page }) => {
    await openFlags(page);
    await page.keyboard.press('j');

    const card = flagCard(page, 'One-page checkout');
    await expect(card.locator('.type')).toHaveText('JSON · 2 variations');
    await expect(card.locator('.line')).toContainText('1 rule, then');

    // One rule and a default: few enough to lay everything out.
    const decides = card.locator('details.decides');
    await expect(decides).toHaveAttribute('open');

    const steps = decides.locator('.steps > li');
    await expect(steps).toHaveCount(2);
    await expect(steps.nth(0)).toContainText('Pilot merchants only');
    await expect(steps.nth(0).locator('.segment-about')).toContainText('Checkout pilot 18 listed by name, 2 excluded');
    await expect(steps.nth(0).locator('.gets .chip')).toHaveText('One page');
    await expect(steps.nth(0).locator('.gets pre')).toContainText('"layout": "one-page"');
    await expect(steps.nth(1).locator('.gets .chip')).toHaveText('Classic');
    await expect(steps.nth(1).locator('.gets pre')).toContainText('"layout": "classic"');
    await expect(steps.nth(0)).toHaveScreenshot('json-rule.png', { maxDiffPixelRatio: 0.01 });

    // Every variation side by side is there for the asking.
    const all = card.locator('details.all-variations');
    await expect(all).not.toHaveAttribute('open');
    await all.locator('> summary').click();
    await expect(all.locator('.variations > li')).toHaveCount(2);
  });

  test('with many rules each JSON result is a line to click', async ({ page }) => {
    await openFlags(page);
    await page.keyboard.press('j');
    await page.keyboard.press('j');

    const card = flagCard(page, 'Payment routing');
    await expect(card.locator('.line')).toContainText('1 named target, 6 rules, then');

    const decides = card.locator('details.decides');
    await expect(decides).not.toHaveAttribute('open');
    await decides.locator('> summary').click();

    const steps = decides.locator('.steps > li');
    await expect(steps).toHaveCount(8);
    await expect(steps.nth(3)).toContainText('if device appVersion is older than 4.2.0');
    await expect(steps.nth(4)).toContainText('if order orderTotal > 5000');
    // A segment LaunchDarkly no longer has is said, not hidden.
    await expect(steps.nth(5).locator('.segment-about').last()).toContainText('deleted-segment couldn’t be read');

    const result = steps.nth(1).locator('.gets details.json');
    await expect(result).not.toHaveAttribute('open');
    await expect(result.locator('.peek')).toContainText('"processor":"stripe"');
    await result.locator('summary').click();
    await expect(result.locator('pre')).toContainText('"manualReview": true');
  });

  test('an off flag says who gets what, and that its rules are idle', async ({ page }) => {
    await openFlags(page);
    await page.getByLabel('env').selectOption('staging');
    await page.keyboard.press('j');
    await page.keyboard.press('j');

    const card = flagCard(page, 'Payment routing');
    await expect(card).toHaveAttribute('data-state', 'off');
    await expect(card.locator('.line .state')).toHaveText('Off');
    await expect(card.locator('.line')).toContainText('everyone gets');
    await expect(card.locator('.line .chip')).toHaveText('Stripe (default)');
    await expect(card.locator('.line')).toContainText('(1 rule set up, not in effect)');
    await expect(card.locator('.steps')).toHaveClass(/idle/);
  });

  test('a flag that is archived, gone or unreadable never breaks the step', async ({ page }) => {
    await openFlags(page);
    await page.keyboard.press('j');
    await page.keyboard.press('j');
    await page.keyboard.press('j');

    await expect(page.locator('[aria-current="step"]')).toContainText('Flags that have gone wrong');
    await expect(page.locator('.flag-card')).toHaveCount(3);

    const archived = flagCard(page, 'Legacy coupon path');
    await expect(archived).toHaveAttribute('data-state', 'archived');
    await expect(archived.locator('.banner')).toContainText('Archived. Nothing evaluates this flag any more.');

    const gone = flagCard(page, 'One-click reorder');
    await expect(gone).toHaveAttribute('data-state', 'not-found');
    await expect(gone.locator('.banner')).toContainText('LaunchDarkly has no flag with this key in default');
    await expect(gone.getByRole('button', { name: 'LaunchDarkly ↗' })).toBeHidden();

    const unreadable = flagCard(page, 'Express pay button');
    await expect(unreadable).toHaveAttribute('data-state', 'unavailable');
    await expect(unreadable.locator('.banner')).toContainText('Status unavailable. LaunchDarkly is rate limiting');
  });

  test('⌘R asks the integration afresh; moving between steps does not', async ({ page }) => {
    await openFlags(page);
    await expect(flagCard(page, 'New checkout flow')).toHaveAttribute('data-state', 'on');
    expect(await page.evaluate(() => window.flagsAsked)).toEqual(['new-checkout-flow']);

    await page.keyboard.press('Meta+r');
    await expect.poll(() => page.evaluate(() => window.flagsAsked)).toContain('new-checkout-flow (fresh)');

    await page.evaluate(() => (window.flagsAsked = []));
    await page.keyboard.press('j');
    await expect(flagCard(page, 'One-page checkout')).toHaveAttribute('data-state', 'on');
    expect(await page.evaluate(() => window.flagsAsked)).toEqual(['one-page-checkout']);
  });

  test('with no token the card says what would turn its status on', async ({ page }) => {
    await page.goto('/');
    await page.evaluate(async () => {
      const fixture = (await (await fetch('/fixture.json')).json()) as NonNullable<typeof window.docentFixture>;
      for (const byFlag of Object.values(fixture.flags)) {
        for (const key of Object.keys(byFlag)) {
          byFlag[key] = {
            ...byFlag[key],
            state: 'no-token',
            reason:
              'Run `docent integrations set launchdarkly`, or set DOCENT_LAUNCHDARKLY_TOKEN, to see this flag’s status',
            targeting: null,
            environments: [],
            variations: [],
            url: '',
          };
        }
      }
      window.docentFixture = fixture;
    });
    await page.getByRole('button', { name: new RegExp(TOUR) }).click();

    const card = flagCard(page, 'New checkout flow');
    await expect(card).toHaveAttribute('data-state', 'no-token');
    await expect(card.locator('.banner')).toContainText('docent integrations set launchdarkly');
    await expect(card.locator('.panel')).toHaveCount(0);
  });

  test('with the integration switched off a flag card is a plain card, and nothing is asked', async ({ page }) => {
    await page.goto('/');
    await page.evaluate(async () => {
      const fixture = (await (await fetch('/fixture.json')).json()) as NonNullable<typeof window.docentFixture>;
      for (const view of [...Object.values(fixture.views), ...Object.values(fixture.picked).map((p) => p.view)]) {
        view.flagStatus = false;
      }
      window.docentFixture = fixture;
    });
    await page.getByRole('button', { name: new RegExp(TOUR) }).click();
    await expect(page.getByRole('heading', { level: 1, name: TOUR })).toBeVisible();

    await expect(page.locator('.flag-card')).toHaveCount(0);

    const cards = page.locator('.links .link-card');
    await expect(cards).toHaveCount(2);
    await expect(cards.nth(0)).toContainText('New checkout flow');
    await expect(cards.nth(0).locator('.where')).toHaveText('new-checkout-flow');
    await expect(cards.nth(0)).toBeDisabled();
    await expect(cards.nth(1)).toBeEnabled();

    expect(await page.evaluate(() => window.flagsAsked)).toEqual([]);
  });
});
