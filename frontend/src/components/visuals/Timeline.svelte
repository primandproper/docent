<script lang="ts">
  import { nodes, timelineFrames } from '../../lib/visuals';
  import type { TimelineVisual } from '../../lib/visuals';
  import Player from './Player.svelte';

  let { visual }: { visual: TimelineVisual } = $props();

  const WIDTH = 560;
  const GUTTER = 96;
  const LANE = 52;
  const PAD = 14;
  const AXIS = 24;

  let frame = $state(0);

  const lanes = $derived(nodes(visual.lanes));
  const events = $derived(timelineFrames(visual));
  const span = $derived(Math.max(1, ...events.map((e) => e.at)));
  const height = $derived(PAD + lanes.length * LANE + AXIS);
  const current = $derived(events[frame]);
  const unit = $derived(visual.unit ? ` ${visual.unit}` : '');

  function x(at: number): number {
    return GUTTER + (at / span) * (WIDTH - GUTTER - PAD - 30);
  }

  function y(lane: string): number {
    return (
      PAD +
      Math.max(
        0,
        lanes.findIndex((l) => l.id === lane),
      ) *
        LANE +
      LANE / 2
    );
  }
</script>

<Player
  kind="timeline"
  count={events.length}
  bind:frame
  title={visual.title}
  caption={current ? `${current.at}${unit}: ${current.label}` : ''}
  step={current?.step}
>
  <svg viewBox={`0 0 ${WIDTH} ${height}`} width={WIDTH} role="img" aria-label={visual.title || 'Timeline of events'}>
    {#each lanes as lane (lane.id)}
      <text class="lane" x={GUTTER - 14} y={y(lane.id)}>{lane.label}</text>
      <line class="track" x1={GUTTER} y1={y(lane.id)} x2={WIDTH - PAD} y2={y(lane.id)} />
    {/each}

    <line class="axis" x1={GUTTER} y1={height - AXIS + 4} x2={WIDTH - PAD} y2={height - AXIS + 4} />
    <text class="tick" x={GUTTER} y={height - 6}>0{unit}</text>
    <text class="tick end" x={x(span)} y={height - 6}>{span}{unit}</text>

    {#if current}
      <line class="now" x1={x(current.at)} y1={PAD} x2={x(current.at)} y2={height - AXIS + 4} />
    {/if}

    {#each events as event, i (i)}
      <!-- Labels alternate above and below their lane so neighbours don't collide. -->
      {@const above = i % 2 === 0}
      <g class="event" class:shown={i <= frame} class:current={i === frame}>
        <circle cx={x(event.at)} cy={y(event.lane)} r="5.5" />
        <text x={x(event.at)} y={y(event.lane) + (above ? -12 : 19)}>{event.label}</text>
      </g>
    {/each}
  </svg>
</Player>

<style>
  .lane {
    fill: var(--ink);
    font-size: 12px;
    font-weight: 600;
    text-anchor: end;
    dominant-baseline: central;
  }

  .track {
    stroke: var(--line);
    stroke-width: 1;
    stroke-dasharray: 2 4;
  }

  .axis {
    stroke: var(--line);
    stroke-width: 1;
  }

  .tick {
    fill: var(--muted);
    font-size: 10.5px;
  }

  .tick.end {
    text-anchor: middle;
  }

  .now {
    stroke: var(--hot);
    stroke-width: 1;
    opacity: 0.6;
    transition:
      x1 var(--ease),
      x2 var(--ease);
  }

  .event {
    opacity: 0;
    transition: opacity var(--ease);
  }

  .event.shown {
    opacity: 0.55;
  }

  .event.current {
    opacity: 1;
  }

  .event circle {
    fill: var(--surface);
    stroke: var(--ink);
    stroke-width: 1.5;
    transition:
      fill var(--ease),
      stroke var(--ease);
  }

  .event.current circle {
    fill: var(--hot);
    stroke: var(--hot);
  }

  .event text {
    fill: var(--ink);
    font-size: 11px;
    text-anchor: middle;
  }
</style>
