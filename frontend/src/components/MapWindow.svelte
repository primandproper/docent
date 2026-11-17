<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import type { MapState } from '../lib/api';
  import MapDiagram from './MapDiagram.svelte';

  // The system map in a window of its own. It keeps nothing but what the main
  // window last told it to draw, and sends the reader's keys and clicks back,
  // so the tour is only ever in one place.
  let shown = $state.raw<MapState | null>(null);
  let heard = $state(false);

  onMount(() => {
    const stop = api.onMap((message) => {
      if (message.kind === 'state') {
        shown = message.state;
        heard = true;
      }
    });
    api.postMap({ kind: 'hello' });
    return stop;
  });

  $effect(() => {
    document.title = shown?.title ? `System map: ${shown.title}` : 'System map';
  });

  function onkeydown(event: KeyboardEvent): void {
    if (event.ctrlKey && !event.metaKey && !event.altKey && event.key.toLowerCase() === 'z') {
      void api.toggleMapWindow();
    } else if (event.metaKey || event.ctrlKey || event.altKey) {
      return;
    } else if (event.key === 'j' || event.key === 'ArrowDown' || event.key === 'ArrowRight') {
      api.postMap({ kind: 'step', by: 1 });
    } else if (event.key === 'k' || event.key === 'ArrowUp' || event.key === 'ArrowLeft') {
      api.postMap({ kind: 'step', by: -1 });
    } else if (event.key === 'z') {
      api.postMap({ kind: 'zoom' });
    } else if (event.key === 'Escape') {
      void api.toggleMapWindow();
    } else {
      return;
    }
    event.preventDefault();
  }
</script>

<svelte:window {onkeydown} />

<main class="map-window" aria-label="System map">
  {#if shown}
    <MapDiagram
      source={shown.map}
      focus={shown.focus}
      index={shown.index}
      zoomed={shown.zoomed}
      fill
      onjump={(index) => api.postMap({ kind: 'go', index })}
    />
  {:else if heard}
    <p class="empty">Open a tour with a system map and it shows here.</p>
  {/if}
</main>

<style>
  .map-window {
    height: 100%;
    padding: 14px;
    background: var(--bg);
  }

  .empty {
    height: 100%;
    margin: 0;
    display: grid;
    place-items: center;
    color: var(--muted);
    font-size: 13px;
    text-align: center;
  }
</style>
