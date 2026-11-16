<script lang="ts">
  import { flowFrames, nodes } from '../../lib/visuals';
  import type { FlowVisual } from '../../lib/visuals';
  import Graph from './Graph.svelte';
  import Player from './Player.svelte';

  let { visual }: { visual: FlowVisual } = $props();

  // How long a packet takes to cross one edge.
  const TRAVEL_MS = 900;

  let frame = $state(0);
  let travel = $state(1);

  const frames = $derived(flowFrames(visual));
  const hop = $derived(frames[frame]);
  const edge = $derived(hop ? visual.edges.findIndex((e) => e.from === hop.from && e.to === hop.to) : -1);

  // Each frame sends the packet along its edge. With reduced motion it simply
  // arrives.
  $effect(() => {
    void frame;
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      travel = 1;
      return;
    }

    const started = performance.now();
    let raf = requestAnimationFrame(function tick(now) {
      const t = Math.min(1, (now - started) / TRAVEL_MS);
      // Ease out, so the packet settles into the node it reaches.
      travel = 1 - (1 - t) ** 3;
      if (t < 1) {
        raf = requestAnimationFrame(tick);
      }
    });
    return () => cancelAnimationFrame(raf);
  });
</script>

<Player
  kind="flow"
  count={frames.length}
  bind:frame
  title={visual.title}
  caption={hop ? `${hop.label}: ${hop.from} → ${hop.to}` : ''}
  step={hop?.step}
>
  <Graph
    nodes={nodes(visual.nodes)}
    edges={visual.edges}
    label={visual.title || 'Flow through a graph'}
    hotNodes={hop ? [travel < 1 ? hop.from : hop.to] : []}
    hotEdge={edge}
    packetEdge={edge}
    {travel}
  />
</Player>
