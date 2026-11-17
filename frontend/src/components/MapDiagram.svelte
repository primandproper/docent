<script lang="ts">
  import { flowchartElements, onSchemeChange, renderDiagram } from '../lib/diagram';
  import type { Rendered } from '../lib/diagram';
  import { between, fit, formatViewBox, parseViewBox, unionAll, zoomTo } from '../lib/zoom';
  import type { Box } from '../lib/zoom';

  // The system map itself, wherever it is shown: docked beside the narrative,
  // large in the centre pane, or in a window of its own.
  let {
    source,
    focus,
    index,
    fill = false,
    zoomed = false,
    onjump,
  }: {
    /** The map's Mermaid source. */
    source: string;
    /** Each step's `mapFocus`, in step order. */
    focus: string[][];
    index: number;
    /** Draw the map as big as its container, instead of at its natural size. */
    fill?: boolean;
    /** Close in on what the current step focuses. */
    zoomed?: boolean;
    /** Called with a step's index when the reader clicks a node that step focuses. */
    onjump?: (index: number) => void;
  } = $props();

  const ZOOM_MS = 350;
  /** Docked, a zoomed map gets at least this much height: a wide, flat diagram leaves nothing to zoom into. */
  const MIN_ZOOMED_HEIGHT = 200;

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
      const first = focus.findIndex((ids) => ids.includes(id));
      for (const el of found) {
        el.classList.add('focusable');
        if (first >= 0 && el.matches('g')) {
          el.classList.add('jumps');
          el.setAttribute('data-jump', String(first));
        } else {
          el.classList.remove('jumps');
          el.removeAttribute('data-jump');
        }
      }
    }
    paint(index);

    const diagram = host;
    const click = (event: MouseEvent) => {
      const jump = event.target instanceof Element ? event.target.closest('[data-jump]') : null;
      if (jump) {
        onjump?.(Number(jump.getAttribute('data-jump')));
      }
    };
    diagram.addEventListener('click', click);
    return () => diagram.removeEventListener('click', click);
  });

  /** Marks what this step focuses, what earlier steps did, and what is still to come. */
  function paint(at: number): void {
    const current = new Set(focus[at] ?? []);
    const visited = new Set(focus.slice(0, at).flat());
    const upcoming = new Set(focus.slice(at + 1).flat());

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
    paint(index);
  });

  // --- Zoom ---
  //
  // Zooming moves the SVG's viewBox, so the diagram is still drawn once and
  // stays sharp. While zoomed the SVG is given a box to fill; the viewBox is
  // what decides how much of the diagram is in it.

  /** The viewBox Mermaid drew the diagram with: all of it. */
  let whole: Box | null = null;
  /** And as Mermaid wrote it, to put back exactly. */
  let wholeText = '';
  /** The viewBox showing now. */
  let showing: Box | null = null;
  let frame = 0;
  let resized = $state(0);

  /** Where what the step focuses is, in the diagram's own coordinates. */
  function focusBox(svg: SVGSVGElement, at: number): Box | null {
    const toDiagram = svg.getScreenCTM()?.inverse();
    if (!toDiagram) {
      return null;
    }

    const boxes: Box[] = [];
    for (const id of focus[at] ?? []) {
      for (const el of elements.get(id) ?? []) {
        const matrix = el instanceof SVGGraphicsElement ? el.getScreenCTM() : null;
        if (!(el instanceof SVGGraphicsElement) || !matrix) {
          continue;
        }
        const local = el.getBBox();
        const toRoot = toDiagram.multiply(matrix);
        const corners = [
          new DOMPoint(local.x, local.y),
          new DOMPoint(local.x + local.width, local.y),
          new DOMPoint(local.x, local.y + local.height),
          new DOMPoint(local.x + local.width, local.y + local.height),
        ].map((corner) => corner.matrixTransform(toRoot));
        const xs = corners.map((corner) => corner.x);
        const ys = corners.map((corner) => corner.y);
        boxes.push({
          x: Math.min(...xs),
          y: Math.min(...ys),
          width: Math.max(...xs) - Math.min(...xs),
          height: Math.max(...ys) - Math.min(...ys),
        });
      }
    }
    return unionAll(boxes);
  }

  function look(svg: SVGSVGElement, box: Box): void {
    showing = box;
    svg.setAttribute('viewBox', formatViewBox(box));
  }

  function travel(svg: SVGSVGElement, to: Box, animate: boolean, then?: () => void): void {
    cancelAnimationFrame(frame);
    const from = showing;
    if (!from || !animate || window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      look(svg, to);
      then?.();
      return;
    }

    const started = performance.now();
    const tick = (now: number) => {
      const t = Math.min(1, (now - started) / ZOOM_MS);
      // Ease in and out.
      look(svg, between(from, to, t < 0.5 ? 2 * t * t : 1 - (-2 * t + 2) ** 2 / 2));
      if (t < 1) {
        frame = requestAnimationFrame(tick);
      } else {
        then?.();
      }
    };
    frame = requestAnimationFrame(tick);
  }

  // A new drawing starts unzoomed, whatever the last one was doing.
  $effect(() => {
    void rendered;
    cancelAnimationFrame(frame);
    whole = null;
    showing = null;
    host?.classList.remove('zoomed');
  });

  $effect(() => {
    const svg = rendered ? host?.querySelector('svg') : null;
    const at = index;
    const resizing = resized;
    void focus;
    if (!host || !svg) {
      return;
    }
    if (!whole) {
      wholeText = svg.getAttribute('viewBox') ?? '';
      whole = parseViewBox(wholeText);
    }
    if (!whole) {
      return;
    }
    const all = whole;
    const diagram = host;
    // A resize is followed at once: easing after the window's edge would lag behind it.
    const animate = resizing === lastResize;
    lastResize = resizing;

    if (!zoomed) {
      if (diagram.classList.contains('zoomed')) {
        const box = svg.getBoundingClientRect();
        travel(svg, fit(all, box.width / box.height), animate, () => {
          diagram.classList.remove('zoomed');
          showing = all;
          svg.setAttribute('viewBox', wholeText);
        });
      }
      return;
    }

    if (!diagram.classList.contains('zoomed')) {
      const before = svg.getBoundingClientRect();
      diagram.style.setProperty('--zoomed-height', `${Math.max(before.height, MIN_ZOOMED_HEIGHT)}px`);
      diagram.classList.add('zoomed');
      // The same picture as before, in the box the SVG now fills.
      const box = svg.getBoundingClientRect();
      look(svg, fit(all, box.width / box.height));
    }
    const box = svg.getBoundingClientRect();
    sizedFor = sizeOf(diagram);
    travel(svg, zoomTo(all, focusBox(svg, at), box.width / box.height), animate);
  });

  let lastResize = 0;
  /** The size the zoom was last worked out for. Zooming changes the size itself; only a change after that is a resize. */
  let sizedFor = '';

  function sizeOf(el: HTMLElement): string {
    return `${el.clientWidth}x${el.clientHeight}`;
  }

  $effect(() => {
    if (!host) {
      return;
    }
    const diagram = host;
    const observer = new ResizeObserver(() => {
      if (diagram.classList.contains('zoomed') && sizeOf(diagram) !== sizedFor) {
        resized++;
      }
    });
    observer.observe(host);
    return () => {
      observer.disconnect();
      cancelAnimationFrame(frame);
    };
  });
</script>

{#if failed}
  <p class="problem">The map couldn't be drawn: {failed}</p>
{/if}
<div class="diagram" class:fill bind:this={host}>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- Mermaid's own output, rendered at securityLevel strict -->
  {@html rendered?.svg ?? ''}
</div>

<style>
  .diagram {
    display: flex;
    justify-content: center;
  }

  .diagram :global(svg) {
    max-width: 100%;
    height: auto;
  }

  .fill {
    height: 100%;
    align-items: center;
  }

  /* Mermaid caps its SVG at the diagram's natural width. Filling means as big as fits. */
  .fill :global(svg) {
    width: 100%;
    height: 100%;
    max-width: none !important;
  }

  .diagram:global(.zoomed) :global(svg) {
    width: 100%;
    height: var(--zoomed-height);
    max-width: none !important;
  }

  .fill:global(.zoomed) :global(svg) {
    height: 100%;
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
