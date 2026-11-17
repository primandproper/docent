<script lang="ts">
  import { session } from '../lib/session.svelte';
  import MapDiagram from './MapDiagram.svelte';

  /** Large: the map has the centre pane, and is drawn as big as fits. */
  let { large = false }: { large?: boolean } = $props();

  const source = $derived(session.view?.map ?? '');
  const focus = $derived((session.view?.tour?.steps ?? []).map((step) => step.mapFocus ?? []));
</script>

{#if source}
  <section class="map" class:large aria-label="System map">
    <div class="controls">
      <button
        aria-pressed={session.mapZoom}
        onclick={() => session.toggleMapZoom()}
        title="Zoom in on what this step is about, or back out (z)"
      >
        Zoom
      </button>
      <button onclick={() => session.toggleMapWindow()} title="Give the map a window of its own (⌃Z)">Pop out</button>
      <button
        onclick={() => session.toggleLargeMap()}
        title={large ? 'Back beside the narrative (⇧M)' : 'Give the map the centre pane (⇧M)'}
      >
        {large ? 'Smaller' : 'Larger'}
      </button>
    </div>
    <MapDiagram
      {source}
      {focus}
      index={session.index}
      fill={large}
      zoomed={session.mapZoom}
      onjump={(index) => void session.go(index)}
    />
  </section>
{:else if session.view?.mapProblem}
  <section class="map" aria-label="System map">
    <p class="problem">{session.view.mapProblem}</p>
  </section>
{/if}

<style>
  .map {
    position: relative;
    flex: none;
    max-height: 42%;
    overflow: auto;
    padding: 10px 12px;
    border-bottom: 1px solid var(--border);
    background: var(--bg);
  }

  .controls {
    position: absolute;
    z-index: 1;
    top: 6px;
    right: 8px;
    display: flex;
    gap: 4px;
    opacity: 0;
    transition: opacity 0.15s ease;
  }

  .controls button {
    padding: 1px 8px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--raised);
    font-size: 11.5px;
    color: var(--muted);
  }

  .controls button[aria-pressed='true'] {
    background: var(--accent-soft);
    border-color: var(--accent);
    color: var(--text);
  }

  /* The controls stay out of the way until the map has the reader's attention. */
  .map:hover .controls,
  .controls:focus-within,
  .large .controls {
    opacity: 1;
  }

  .map.large {
    max-height: none;
    height: 100%;
    min-width: 0;
    padding: 20px;
    border-bottom: none;
    border-right: 1px solid var(--border);
  }

  .problem {
    margin: 0;
    font-size: 12.5px;
    color: var(--amber);
  }
</style>
