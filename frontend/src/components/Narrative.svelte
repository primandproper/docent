<script lang="ts">
  import { api, isExternalURL } from '../lib/api';
  import { session } from '../lib/session.svelte';
  import { toneOf } from '../lib/status';
  import { stepTitle } from '../lib/steps';
  import LinkCards from './LinkCards.svelte';
  import Markdown from './Markdown.svelte';
  import Visual from './visuals/Visual.svelte';

  let { wide = false }: { wide?: boolean } = $props();

  const tour = $derived(session.view?.tour);
  const step = $derived(tour?.steps?.[session.index]);
  const resolved = $derived(session.step?.index === session.index ? session.step : null);
  const location = $derived(resolved?.location ?? null);
  const isLast = $derived(session.index === session.stepCount - 1);
  const nextTour = $derived(session.nextTour);

  let body: HTMLDivElement | undefined = $state();

  // A new step starts at the top of its text.
  $effect(() => {
    void session.index;
    void session.path;
    body?.scrollTo({ top: 0 });
  });
</script>

{#if step}
  <article class="narrative" class:wide>
    <div class="body" bind:this={body}>
      <div class="column">
        <p class="count">Step {session.index + 1} of {session.stepCount}</p>
        <h2 class="selectable">{stepTitle(step)}</h2>

        {#if location?.problem}
          <p class="notice {toneOf(location.status)}" role="status">{location.detail}</p>
        {/if}

        {#if resolved?.removed}
          <p class="notice red">
            Removed in <code>{resolved.removed.sha.slice(0, 7)}</code>: {resolved.removed.subject}
          </p>
        {/if}

        {#if resolved?.cloneCommand}
          <div class="clone">
            <code class="selectable">{resolved.cloneCommand}</code>
            <button onclick={() => session.copy(resolved.cloneCommand, 'the clone command')}>Copy</button>
          </div>
        {/if}

        {#if resolved?.bodyProblem}
          <p class="notice red" role="status">{resolved.bodyProblem}</p>
        {/if}

        {#if step.visual}
          <Visual value={step.visual} />
        {/if}

        <!-- The description stands in until the body arrives, and when there is none. -->
        <Markdown source={resolved?.body || step.description} />

        <LinkCards cards={resolved?.links ?? []} />

        {#if step.uri && !step.file && !step.directory}
          {#if isExternalURL(step.uri)}
            <button class="link-card" onclick={() => api.openExternal(step.uri)}>
              <span class="host">{new URL(step.uri).host}</span>
              <span class="url">{step.uri}</span>
            </button>
          {:else}
            <p class="notice amber">This step links to <code>{step.uri}</code>, which Docent doesn't open.</p>
          {/if}
        {/if}
      </div>
    </div>

    <footer>
      <button class="nav" disabled={session.index === 0} onclick={() => session.previous()}>
        <kbd>k</kbd> Previous
      </button>
      <span class="spacer"></span>
      {#if !isLast}
        <button class="nav primary" onclick={() => session.next()}>Next <kbd>j</kbd></button>
      {:else if nextTour}
        <button class="nav primary" onclick={() => session.open(nextTour.path)}>Next tour: {nextTour.title}</button>
      {:else}
        <button class="nav" onclick={() => session.close()}>Back to tours</button>
      {/if}
    </footer>
  </article>
{/if}

<style>
  .narrative {
    height: 100%;
    display: flex;
    flex-direction: column;
    min-width: 0;
    background: var(--bg);
  }

  .body {
    flex: 1;
    overflow-y: auto;
    padding: 20px 22px 24px;
  }

  .wide .body {
    padding: 40px 32px;
  }

  .wide .column {
    max-width: 720px;
    margin: 0 auto;
    font-size: 15px;
  }

  .count {
    margin: 0 0 2px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--faint);
  }

  h2 {
    margin: 0 0 14px;
    font-size: 19px;
    line-height: 1.3;
    font-weight: 650;
    letter-spacing: -0.005em;
    overflow-wrap: anywhere;
  }

  .wide h2 {
    font-size: 24px;
  }

  .notice {
    margin: 0 0 14px;
    padding: 8px 11px;
    border-radius: 8px;
    font-size: 13px;
  }

  .notice.amber,
  .notice.ok {
    background: var(--amber-soft);
    color: var(--amber);
  }

  .notice.red {
    background: var(--red-soft);
    color: var(--red);
  }

  .notice code {
    font-family: var(--mono);
    font-size: 12px;
  }

  .link-card {
    margin-top: 6px;
    width: 100%;
    text-align: left;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 11px 13px;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--raised);
  }

  .link-card:hover {
    border-color: var(--accent);
  }

  .clone {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0 0 14px;
    padding: 8px 8px 8px 11px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--panel);
  }

  .clone code {
    flex: 1;
    font-family: var(--mono);
    font-size: 12px;
    overflow-wrap: anywhere;
  }

  .clone button {
    flex: none;
    padding: 3px 9px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--raised);
    font-size: 12px;
  }

  .host {
    font-weight: 600;
  }

  .url {
    font-family: var(--mono);
    font-size: 11.5px;
    color: var(--muted);
    overflow-wrap: anywhere;
  }

  footer {
    flex: none;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 14px;
    border-top: 1px solid var(--border);
    background: var(--panel);
  }

  .spacer {
    flex: 1;
  }

  .nav {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    padding: 5px 11px;
    border-radius: 7px;
    border: 1px solid var(--border);
    background: var(--raised);
    font-size: 13px;
  }

  .nav:hover:not(:disabled) {
    border-color: var(--accent);
  }

  .nav:disabled {
    opacity: 0.45;
  }

  .nav.primary {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--bg);
    font-weight: 600;
  }

  .nav.primary kbd {
    background: transparent;
    border-color: color-mix(in srgb, var(--bg) 45%, transparent);
    color: var(--bg);
  }
</style>
