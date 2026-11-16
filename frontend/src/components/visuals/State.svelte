<script lang="ts">
  import { nodes, stateFrames } from '../../lib/visuals';
  import type { StateVisual } from '../../lib/visuals';
  import Graph from './Graph.svelte';
  import Player from './Player.svelte';

  let { visual }: { visual: StateVisual } = $props();

  let frame = $state(0);

  const states = $derived(nodes(visual.states));
  // Frame 0 is the machine at rest in its first state; each later frame is
  // one event of the run.
  const transitions = $derived(stateFrames(visual));
  const taken = $derived(frame > 0 ? transitions[frame - 1] : null);
  const current = $derived(taken ? (taken.to ?? taken.from) : (states[0]?.id ?? ''));
  const edge = $derived(
    taken?.to
      ? visual.transitions.findIndex((t) => t.from === taken.from && t.to === taken.to && t.on === taken.event)
      : -1,
  );

  const caption = $derived.by(() => {
    if (!taken) {
      return `Starts in ${current}`;
    }
    return taken.to
      ? `${taken.event}: ${taken.from} → ${taken.to}`
      : `${taken.event}: no transition from ${taken.from}`;
  });
</script>

<Player kind="state" count={transitions.length + 1} bind:frame title={visual.title} {caption}>
  <Graph
    nodes={states}
    edges={visual.transitions.map((t) => ({ from: t.from, to: t.to, label: t.on }))}
    label={visual.title || 'State machine'}
    hotNodes={[current]}
    hotEdge={edge}
  />
</Player>
