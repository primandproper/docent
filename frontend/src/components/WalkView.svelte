<script lang="ts">
  import { session } from '../lib/session.svelte';
  import { statusLabel, toneOf } from '../lib/status';
  import CodePane from './CodePane.svelte';
  import DirectoryPane from './DirectoryPane.svelte';
  import Narrative from './Narrative.svelte';
  import StepRail from './StepRail.svelte';

  const tour = $derived(session.view?.tour);
  const diagnostics = $derived(session.view?.diagnostics ?? []);
  // The step view lags the index by one round trip; only show it once it matches.
  const step = $derived(session.step?.index === session.index ? session.step : null);
  const kind = $derived(session.view?.steps?.[session.index]?.kind ?? 'content');
  const hasPane = $derived(kind === 'file' || kind === 'directory');
  const unreviewed = $derived(!!tour?.generatedBy && (tour.reviewedBy ?? []).length === 0);

  let showProblems = $state(false);
</script>

<div class="walk">
  <header class="bar">
    <button class="back" onclick={() => session.close()} title="Back to tours (Esc)">‹ Tours</button>
    <h1>{tour?.title}</h1>
    {#if unreviewed}
      <span class="badge amber">Unreviewed draft</span>
    {/if}
    <span class="spacer"></span>
    {#if diagnostics.length > 0}
      <button class="badge red" onclick={() => (showProblems = !showProblems)} aria-expanded={showProblems}>
        {diagnostics.length}
        {diagnostics.length === 1 ? 'problem' : 'problems'} in this file
      </button>
    {/if}
    <span class="hint"><kbd>j</kbd><kbd>k</kbd> step <kbd>g</kbd> go to <kbd>⌘R</kbd> reload</span>
  </header>

  {#if showProblems && diagnostics.length > 0}
    <ul class="problems selectable">
      {#each diagnostics as d, i (i)}
        <li>
          <span class="severity {d.severity === 'error' ? 'red' : 'amber'}">{d.severity}</span>
          <code>{d.code}</code>
          {#if d.pointer}<code class="pointer">{d.pointer}</code>{/if}
          <span>{d.message}</span>
        </li>
      {/each}
    </ul>
  {/if}

  <div class="panes" class:no-pane={!hasPane}>
    <StepRail />

    {#if hasPane}
      <section class="pane">
        <div class="crumbs">
          <span class="crumb root">{session.workspace?.name}</span>
          {#each (step?.path ?? '').split('/').filter(Boolean) as part, i (i)}
            <span class="sep">›</span><span class="crumb">{part}</span>
          {/each}
          {#if step && step.location.range.startLine > 0}
            <span class="sep">›</span>
            <span class="crumb line">
              {step.location.range.startLine === step.location.range.endLine
                ? `line ${step.location.range.startLine}`
                : `lines ${step.location.range.startLine}–${step.location.range.endLine}`}
            </span>
          {/if}
          <span class="spacer"></span>
          {#if step && statusLabel(step.location)}
            <span class="badge {toneOf(step.location.status)}" title={step.location.detail}>
              {statusLabel(step.location)}
            </span>
          {/if}
        </div>

        <div class="content">
          {#if !step}
            <div class="placeholder"></div>
          {:else if step.kind === 'directory'}
            {#if step.location.problem}
              <div class="placeholder">{step.location.detail}</div>
            {:else}
              <DirectoryPane path={step.path} entries={step.entries ?? []} readme={step.readme} />
            {/if}
          {:else if step.unavailable}
            <div class="placeholder">{step.unavailable}</div>
          {:else if step.content === '' && step.location.status === 'missing'}
            <div class="placeholder">{step.location.detail}</div>
          {:else}
            <CodePane path={step.path} content={step.content} range={step.location.range} />
          {/if}
        </div>
      </section>
    {/if}

    <Narrative wide={!hasPane} />
  </div>
</div>

<style>
  .walk {
    height: 100%;
    display: flex;
    flex-direction: column;
  }

  .bar {
    flex: none;
    display: flex;
    align-items: center;
    gap: 12px;
    height: 44px;
    padding: 0 14px;
    border-bottom: 1px solid var(--border);
    background: var(--panel);
  }

  .back {
    color: var(--muted);
    padding: 3px 8px;
    border-radius: 6px;
    font-size: 13px;
  }

  .back:hover {
    background: var(--hover);
    color: var(--text);
  }

  h1 {
    margin: 0;
    font-size: 14px;
    font-weight: 650;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .spacer {
    flex: 1;
  }

  .hint {
    color: var(--faint);
    font-size: 12px;
    white-space: nowrap;
  }

  .hint kbd {
    margin-left: 8px;
  }

  .hint kbd + kbd {
    margin-left: 2px;
  }

  .badge {
    font-size: 11px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 999px;
    white-space: nowrap;
  }

  .badge.amber,
  .badge.ok {
    background: var(--amber-soft);
    color: var(--amber);
  }

  .badge.red {
    background: var(--red-soft);
    color: var(--red);
  }

  .problems {
    flex: none;
    list-style: none;
    margin: 0;
    padding: 8px 14px;
    max-height: 30vh;
    overflow-y: auto;
    border-bottom: 1px solid var(--border);
    font-size: 12.5px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .problems li {
    display: flex;
    gap: 8px;
    align-items: baseline;
  }

  .problems code {
    font-family: var(--mono);
    font-size: 11.5px;
    color: var(--muted);
  }

  .severity {
    font-weight: 600;
    font-size: 11px;
    text-transform: uppercase;
  }

  .severity.red {
    color: var(--red);
  }

  .severity.amber {
    color: var(--amber);
  }

  .panes {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: 250px minmax(0, 1fr) minmax(320px, 30%);
  }

  .panes.no-pane {
    grid-template-columns: 250px minmax(0, 1fr);
  }

  .pane {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    border-right: 1px solid var(--border);
  }

  .crumbs {
    flex: none;
    display: flex;
    align-items: center;
    gap: 6px;
    height: 34px;
    padding: 0 14px;
    border-bottom: 1px solid var(--border);
    font-family: var(--mono);
    font-size: 12px;
    color: var(--muted);
    white-space: nowrap;
    overflow: hidden;
  }

  .crumb.root {
    font-weight: 600;
    color: var(--text);
  }

  .crumb.line {
    color: var(--accent);
  }

  .sep {
    color: var(--faint);
  }

  .content {
    flex: 1;
    min-height: 0;
  }

  .placeholder {
    height: 100%;
    display: grid;
    place-items: center;
    padding: 24px;
    color: var(--muted);
    text-align: center;
  }
</style>
