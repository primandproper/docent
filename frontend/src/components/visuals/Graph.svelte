<script lang="ts">
  import { layoutGraph, pointAt, toPath } from '../../lib/layout';
  import type { Layout } from '../../lib/layout';
  import type { Node } from '../../lib/visuals';

  let {
    nodes,
    edges,
    label,
    hotNodes = [],
    hotEdge = -1,
    packetEdge = -1,
    travel = 0,
  }: {
    nodes: Node[];
    edges: { from: string; to: string; label?: string }[];
    label: string;
    /** Node ids to highlight. */
    hotNodes?: string[];
    /** The index of the edge to highlight, or -1. */
    hotEdge?: number;
    /** The index of the edge a packet is on, or -1 for no packet. */
    packetEdge?: number;
    /** How far along its edge the packet is, 0 to 1. */
    travel?: number;
  } = $props();

  let layout = $state.raw<Layout | null>(null);
  let failed = $state('');

  $effect(() => {
    let stale = false;
    layoutGraph(nodes, edges)
      .then((result) => {
        if (!stale) {
          layout = result;
        }
      })
      .catch((err: unknown) => {
        if (!stale) {
          failed = err instanceof Error ? err.message : String(err);
        }
      });
    return () => {
      stale = true;
    };
  });

  const packet = $derived(layout && packetEdge >= 0 ? pointAt(layout.edges[packetEdge]?.points ?? [], travel) : null);
</script>

{#if failed}
  <p class="failed">This graph couldn't be laid out: {failed}</p>
{:else if layout}
  <svg viewBox={`0 0 ${layout.width} ${layout.height}`} width={layout.width} role="img" aria-label={label}>
    <defs>
      <!-- Two markers, not context-stroke: WebKit doesn't support it, and the app runs in WebKit. -->
      {#each ['', '-hot'] as variant (variant)}
        <marker
          id={`graph-arrow${variant}`}
          viewBox="0 0 10 10"
          refX="9"
          refY="5"
          markerUnits="userSpaceOnUse"
          markerWidth="9"
          markerHeight="9"
          orient="auto"
        >
          <path class={`head${variant}`} d="M0,0 L10,5 L0,10 z" />
        </marker>
      {/each}
    </defs>

    {#each layout.edges as edge, i (i)}
      <g class="edge" class:hot={i === hotEdge} data-edge={`${edge.from}->${edge.to}`}>
        <path d={toPath(edge.points)} marker-end={i === hotEdge ? 'url(#graph-arrow-hot)' : 'url(#graph-arrow)'} />
        {#if edge.label}
          <text x={edge.at.x} y={edge.at.y}>{edge.label}</text>
        {/if}
      </g>
    {/each}

    {#each layout.nodes as node (node.id)}
      <g class="node" class:hot={hotNodes.includes(node.id)} data-node={node.id}>
        <rect x={node.x} y={node.y} width={node.width} height={node.height} rx="7" />
        <text x={node.x + node.width / 2} y={node.y + node.height / 2}>{node.label}</text>
      </g>
    {/each}

    {#if packet}
      <circle class="packet" cx={packet.x} cy={packet.y} r="6" />
    {/if}
  </svg>
{:else}
  <div class="waiting"></div>
{/if}

<style>
  .failed {
    margin: 0;
    font-size: 12.5px;
    color: var(--amber);
  }

  .waiting {
    height: 90px;
  }

  marker .head {
    fill: var(--line);
  }

  marker .head-hot {
    fill: var(--hot);
  }

  .edge path {
    fill: none;
    stroke: var(--line);
    stroke-width: 1.5;
    transition:
      stroke var(--ease),
      stroke-width var(--ease);
  }

  .edge.hot path {
    stroke: var(--hot);
    stroke-width: 2.5;
  }

  .edge text {
    fill: var(--muted);
    font-size: 11px;
    text-anchor: middle;
    dominant-baseline: central;
    paint-order: stroke;
    stroke: var(--panel);
    stroke-width: 4px;
  }

  .node rect {
    fill: var(--surface);
    stroke: var(--line);
    stroke-width: 1.2;
    transition:
      fill var(--ease),
      stroke var(--ease);
  }

  .node.hot rect {
    fill: var(--hot-soft);
    stroke: var(--hot);
    stroke-width: 2;
  }

  .node text {
    fill: var(--ink);
    font-size: 12px;
    font-weight: 600;
    text-anchor: middle;
    dominant-baseline: central;
  }

  .packet {
    fill: var(--hot);
    stroke: var(--panel);
    stroke-width: 2;
  }
</style>
