<script lang="ts">
  import { tick } from 'svelte';
  import { basename, stepTitle } from '../lib/steps';
  import { session } from '../lib/session.svelte';
  import { toneOf } from '../lib/status';

  const steps = $derived(session.view?.tour?.steps ?? []);
  const summaries = $derived(session.view?.steps ?? []);

  let list: HTMLOListElement | undefined = $state();

  // Keep the current step in view when the keyboard moves it.
  $effect(() => {
    const index = session.index;
    void tick().then(() => {
      list?.querySelector(`[data-index="${index}"]`)?.scrollIntoView({ block: 'nearest' });
    });
  });
</script>

<nav class="rail" aria-label="Steps">
  <ol bind:this={list}>
    {#each steps as step, i (i)}
      {@const tone = toneOf(summaries[i]?.status ?? 'ok')}
      {@const target = step.file || step.directory}
      <li>
        <button
          class="step"
          class:current={i === session.index}
          data-index={i}
          aria-current={i === session.index ? 'step' : undefined}
          onclick={() => session.go(i)}
        >
          <span class="number">{i + 1}</span>
          <span class="text">
            <span class="title">{stepTitle(step)}</span>
            {#if target && step.title}
              <span class="target">{basename(target)}{step.directory && !step.file ? '/' : ''}</span>
            {/if}
          </span>
          {#if tone !== 'ok'}
            <span class="dot {tone}" title={summaries[i]?.status}></span>
          {/if}
        </button>
      </li>
    {/each}
  </ol>
</nav>

<style>
  .rail {
    height: 100%;
    overflow-y: auto;
    background: var(--panel);
    border-right: 1px solid var(--border);
  }

  ol {
    list-style: none;
    margin: 0;
    padding: 8px;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .step {
    width: 100%;
    text-align: left;
    display: flex;
    align-items: flex-start;
    gap: 9px;
    padding: 7px 8px;
    border-radius: 7px;
  }

  .step:hover {
    background: var(--hover);
  }

  .step.current {
    background: var(--accent-soft);
  }

  .number {
    flex: none;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    font-size: 11px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    color: var(--muted);
    border: 1px solid var(--border);
    background: var(--raised);
  }

  .current .number {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--bg);
  }

  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .title {
    font-size: 13px;
    line-height: 1.35;
    padding-top: 2px;
    overflow-wrap: anywhere;
  }

  .current .title {
    font-weight: 600;
  }

  .target {
    font-family: var(--mono);
    font-size: 11px;
    color: var(--faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .dot {
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-top: 8px;
  }

  .dot.amber {
    background: var(--amber);
  }

  .dot.red {
    background: var(--red);
  }
</style>
