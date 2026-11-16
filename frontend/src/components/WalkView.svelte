<script lang="ts">
  import { session } from '../lib/session.svelte';
  import CodeArea from './CodeArea.svelte';
  import Freshness from './Freshness.svelte';
  import Narrative from './Narrative.svelte';
  import ReviewButton from './ReviewButton.svelte';
  import StepRail from './StepRail.svelte';
  import SystemMap from './SystemMap.svelte';

  const tour = $derived(session.view?.tour);
  const diagnostics = $derived(session.view?.diagnostics ?? []);
  const kind = $derived(session.view?.steps?.[session.index]?.kind ?? 'content');
  const layout = $derived(tour?.steps?.[session.index]?.layout ?? '');
  const hasAnchors = $derived((tour?.steps?.[session.index]?.anchors ?? []).length > 0);
  // A step's `layout` can override what its content would get: `narrative`
  // hides the code, `code` hides the narrative.
  const hasPane = $derived(layout !== 'narrative' && (kind === 'file' || kind === 'directory' || hasAnchors));
  const onlyCode = $derived(layout === 'code' && hasPane);
  // Only a tour with a map can show one large.
  const mapLarge = $derived(session.mapLarge && session.mapShown && !!session.view?.map);
  const unreviewed = $derived(!!tour?.generatedBy && (tour.reviewedBy ?? []).length === 0);
  const reviewers = $derived((tour?.reviewedBy ?? []).map((review) => review.name));

  let showProblems = $state(false);
</script>

<div class="walk">
  <header class="bar">
    <button class="back" onclick={() => session.close()} title="Back to tours (Esc)">‹ Tours</button>
    <h1>{tour?.title}</h1>
    {#if unreviewed}
      <span class="badge amber">Unreviewed draft</span>
    {:else if reviewers.length > 0}
      <span class="badge green" title={`Reviewed by ${reviewers.join(', ')}`}>
        Reviewed by {reviewers[reviewers.length - 1]}
      </span>
    {/if}
    <ReviewButton />
    <span class="spacer"></span>
    {#each session.vars as v (v.name)}
      {@const choices = v.choices ?? []}
      <label class="var" title={v.locked ? 'Set with --var on the command line' : v.description}>
        <span>{v.name}</span>
        {#if choices.length > 0}
          <select value={v.value} disabled={v.locked} onchange={(e) => session.setVar(v.name, e.currentTarget.value)}>
            {#if !choices.includes(v.value)}<option value={v.value}>{v.value || '(unset)'}</option>{/if}
            {#each choices as choice (choice)}
              <option value={choice}>{choice}</option>
            {/each}
          </select>
        {:else}
          <input
            value={v.value}
            disabled={v.locked}
            placeholder="unset"
            spellcheck="false"
            autocomplete="off"
            onchange={(e) => session.setVar(v.name, e.currentTarget.value.trim())}
          />
        {/if}
      </label>
    {/each}
    {#if session.view?.map || session.view?.mapProblem}
      <button
        class="toggle"
        aria-pressed={session.mapShown}
        onclick={() => (session.mapShown = !session.mapShown)}
        title="Show or hide the system map (m)"
      >
        Map
      </button>
    {/if}
    <button
      class="toggle"
      aria-pressed={session.view?.workingTree ?? false}
      onclick={() => session.setWorkingTree(!session.view?.workingTree)}
      title="Read files as they are on disk, uncommitted changes included, instead of at each repo's ref"
    >
      Working tree
    </button>
    {#if diagnostics.length > 0}
      <button class="badge red" onclick={() => (showProblems = !showProblems)} aria-expanded={showProblems}>
        {diagnostics.length}
        {diagnostics.length === 1 ? 'problem' : 'problems'} in this file
      </button>
    {/if}
    <span class="hint"><kbd>j</kbd><kbd>k</kbd> step <kbd>g</kbd> go to <kbd>o</kbd> open</span>
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

  <Freshness />

  <div class="panes" class:no-pane={!hasPane && !mapLarge} class:only-code={onlyCode && !mapLarge}>
    <StepRail />

    <!-- Large, the map takes the centre pane from the code and the narrative stays beside it. -->
    {#if mapLarge}
      <SystemMap large />
    {:else if hasPane}
      <CodeArea />
    {/if}

    <div class="reading" hidden={onlyCode && !mapLarge}>
      {#if session.mapShown && !mapLarge}
        <SystemMap />
      {/if}
      <Narrative wide={!hasPane && !mapLarge} />
    </div>
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

  .var {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: 12px;
    color: var(--muted);
  }

  .var select,
  .var input {
    font: inherit;
    font-size: 12px;
    color: var(--text);
    background: var(--raised);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 2px 6px;
    max-width: 130px;
  }

  .toggle {
    font-size: 12px;
    padding: 2px 9px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--raised);
    color: var(--muted);
  }

  .toggle[aria-pressed='true'] {
    background: var(--accent-soft);
    border-color: var(--accent);
    color: var(--text);
  }

  .badge {
    font-size: 11px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 999px;
    white-space: nowrap;
  }

  .badge.amber {
    background: var(--amber-soft);
    color: var(--amber);
  }

  .badge.green {
    background: var(--green-soft);
    color: var(--green);
    max-width: 220px;
    overflow: hidden;
    text-overflow: ellipsis;
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

  .panes.only-code {
    grid-template-columns: 250px minmax(0, 1fr);
  }

  .panes.no-pane {
    grid-template-columns: 250px minmax(0, 1fr);
  }

  .reading {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }

  .reading[hidden] {
    display: none;
  }

  .reading > :global(.narrative) {
    flex: 1;
    min-height: 0;
    height: auto;
  }
</style>
