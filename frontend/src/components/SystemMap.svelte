<script lang="ts">
  import { flowchartElements, onSchemeChange, renderDiagram } from '../lib/diagram';
  import type { Rendered } from '../lib/diagram';
  import { session } from '../lib/session.svelte';

  const source = $derived(session.view?.map ?? '');
  const steps = $derived(session.view?.tour?.steps ?? []);

  let host: HTMLDivElement | undefined = $state();
  let rendered = $state.raw<Rendered | null>(null);
  let failed = $state('');
  let elements = new Map<string, Element[]>();

  // The map is drawn once per tour. Moving between steps only swaps classes
  // on what is already there, which is what lets the highlight travel.
  let scheme = $state(0);

  $effect(() => onSchemeChange(() => scheme++));

  $effect(() => {
    const text = source;
    void scheme;
    let stale = false;

    rendered = null;
    failed = '';

    if (text) {
      renderDiagram(text)
        .then((result) => {
          if (!stale) {
            rendered = result;
          }
        })
        .catch((err: unknown) => {
          if (!stale) {
            failed = err instanceof Error ? err.message : String(err);
          }
        });
    }

    return () => {
      stale = true;
    };
  });

  $effect(() => {
    if (!host || !rendered) {
      return;
    }
    // Effects run after the DOM is updated, so the SVG below is in place.
    elements = flowchartElements(host, rendered.id);

    for (const [id, found] of elements) {
      const first = steps.findIndex((step) => (step.mapFocus ?? []).includes(id));
      for (const el of found) {
        el.classList.add('focusable');
        if (first >= 0 && el.matches('g')) {
          el.classList.add('jumps');
          el.addEventListener('click', () => void session.go(first));
        }
      }
    }
    paint(session.index);
  });

  /** Marks what this step focuses, what earlier steps did, and what is still to come. */
  function paint(index: number): void {
    const current = new Set(steps[index]?.mapFocus ?? []);
    const visited = new Set(steps.slice(0, index).flatMap((step) => step.mapFocus ?? []));
    const upcoming = new Set(steps.slice(index + 1).flatMap((step) => step.mapFocus ?? []));

    for (const [id, found] of elements) {
      for (const el of found) {
        el.classList.toggle('current', current.has(id));
        el.classList.toggle('visited', !current.has(id) && visited.has(id));
        el.classList.toggle('upcoming', !current.has(id) && !visited.has(id) && upcoming.has(id));
      }
    }
  }

  $effect(() => {
    void rendered;
    paint(session.index);
  });
</script>

{#if source}
  <section class="map" aria-label="System map">
    {#if failed}
      <p class="problem">The map couldn't be drawn: {failed}</p>
    {/if}
    <div class="diagram" bind:this={host}>
      <!-- eslint-disable-next-line svelte/no-at-html-tags -- Mermaid's own output, rendered at securityLevel strict -->
      {@html rendered?.svg ?? ''}
    </div>
  </section>
{:else if session.view?.mapProblem}
  <section class="map" aria-label="System map">
    <p class="problem">{session.view.mapProblem}</p>
  </section>
{/if}

<style>
  .map {
    flex: none;
    max-height: 42%;
    overflow: auto;
    padding: 10px 12px;
    border-bottom: 1px solid var(--border);
    background: var(--bg);
  }

  .diagram {
    display: flex;
    justify-content: center;
  }

  .diagram :global(svg) {
    max-width: 100%;
    height: auto;
  }

  .problem {
    margin: 0;
    font-size: 12.5px;
    color: var(--amber);
  }

  /* Everything a step can focus fades back, so the current step stands out. */
  .diagram :global(.focusable) {
    transition:
      opacity 0.35s ease,
      filter 0.35s ease;
  }

  .diagram :global(.focusable :is(rect, polygon, circle, ellipse, path)),
  .diagram :global(path.focusable) {
    transition:
      stroke 0.35s ease,
      stroke-width 0.35s ease,
      fill 0.35s ease;
  }

  .diagram :global(.jumps) {
    cursor: pointer;
  }

  .diagram :global(.visited) {
    opacity: 0.55;
  }

  .diagram :global(g.current :is(rect, polygon, circle, ellipse, path)) {
    stroke: var(--accent) !important;
    stroke-width: 2.5px !important;
    fill: var(--accent-soft) !important;
  }

  .diagram :global(path.current) {
    stroke: var(--accent) !important;
    stroke-width: 3px !important;
  }

  @media (prefers-reduced-motion: reduce) {
    .diagram :global(*) {
      transition: none !important;
    }
  }
</style>
