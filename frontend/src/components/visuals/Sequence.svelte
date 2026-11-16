<script lang="ts">
  import { nodes } from '../../lib/visuals';
  import type { SequenceVisual } from '../../lib/visuals';
  import Player from './Player.svelte';

  let { visual }: { visual: SequenceVisual } = $props();

  const COLUMN = 118;
  const HEAD = 30;
  const ROW = 38;
  const PAD = 12;

  let frame = $state(0);

  const participants = $derived(nodes(visual.participants));
  const column = $derived(new Map(participants.map((p, i) => [p.id, PAD + COLUMN / 2 + i * COLUMN])));
  const width = $derived(PAD * 2 + participants.length * COLUMN);
  const height = $derived(PAD * 2 + HEAD + visual.messages.length * ROW + 14);
  const current = $derived(visual.messages[frame]);

  function x(id: string): number {
    return column.get(id) ?? PAD;
  }
</script>

<Player
  kind="sequence"
  count={visual.messages.length}
  bind:frame
  title={visual.title}
  caption={current ? `${current.from} → ${current.to}: ${current.label}` : ''}
  step={current?.step}
>
  <svg viewBox={`0 0 ${width} ${height}`} {width} role="img" aria-label={visual.title || 'Sequence of messages'}>
    <defs>
      <!-- Two markers, not context-stroke: WebKit doesn't support it, and the app runs in WebKit. -->
      {#each ['', '-hot'] as variant (variant)}
        <marker
          id={`seq-arrow${variant}`}
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

    {#each participants as p (p.id)}
      {@const active = current && (current.from === p.id || current.to === p.id)}
      <line class="lifeline" x1={x(p.id)} y1={PAD + HEAD} x2={x(p.id)} y2={height - PAD} />
      <rect class="box" class:active x={x(p.id) - COLUMN / 2 + 8} y={PAD} width={COLUMN - 16} height={HEAD} rx="6" />
      <text class="name" x={x(p.id)} y={PAD + HEAD / 2}>{p.label}</text>
    {/each}

    {#each visual.messages as m, i (i)}
      {@const y = PAD + HEAD + (i + 1) * ROW - 6}
      {@const from = x(m.from)}
      {@const to = x(m.to)}
      {@const arrow = i === frame ? 'url(#seq-arrow-hot)' : 'url(#seq-arrow)'}
      <g class="message" class:shown={i <= frame} class:current={i === frame} class:async={m.async}>
        {#if m.from === m.to}
          <path d={`M${from},${y - 12} h26 v14 h-22`} marker-end={arrow} />
          <text class="label self" x={from + 32} y={y - 5}>{m.label}</text>
        {:else}
          <line x1={from} y1={y} x2={to + (to > from ? -2 : 2)} y2={y} marker-end={arrow} />
          <text class="label" x={(from + to) / 2} y={y - 7}>{m.label}</text>
        {/if}
      </g>
    {/each}
  </svg>
</Player>

<style>
  marker .head {
    fill: var(--ink);
    stroke: none;
  }

  marker .head-hot {
    fill: var(--hot);
    stroke: none;
  }

  .lifeline {
    stroke: var(--line);
    stroke-width: 1;
    stroke-dasharray: 3 4;
  }

  .box {
    fill: var(--surface);
    stroke: var(--line);
    transition:
      fill var(--ease),
      stroke var(--ease);
  }

  .box.active {
    fill: var(--hot-soft);
    stroke: var(--hot);
  }

  .name {
    fill: var(--ink);
    font-size: 12px;
    font-weight: 600;
    text-anchor: middle;
    dominant-baseline: central;
  }

  .message {
    opacity: 0;
    transition: opacity var(--ease);
  }

  .message.shown {
    opacity: 0.5;
  }

  .message.current {
    opacity: 1;
  }

  .message line,
  .message path {
    fill: none;
    stroke: var(--ink);
    stroke-width: 1.5;
    transition: stroke var(--ease);
  }

  .message.current line,
  .message.current path {
    stroke: var(--hot);
    stroke-width: 2;
  }

  .message.async line,
  .message.async path {
    stroke-dasharray: 6 4;
  }

  .label {
    fill: var(--ink);
    font-size: 11.5px;
    text-anchor: middle;
  }

  .label.self {
    text-anchor: start;
  }
</style>
