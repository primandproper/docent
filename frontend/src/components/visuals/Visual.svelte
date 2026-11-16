<script lang="ts">
  import { asVisual } from '../../lib/visuals';
  import Flow from './Flow.svelte';
  import Sequence from './Sequence.svelte';
  import State from './State.svelte';
  import Timeline from './Timeline.svelte';

  /** A visual as the tour declared it: a decoded object, not yet trusted to be well-formed. */
  let { value, problem = '' }: { value: unknown; problem?: string } = $props();

  const visual = $derived(asVisual(value));
</script>

{#if problem || !visual}
  <p class="broken" role="status">
    This visual can't be shown: {problem || 'it is missing fields its type needs'}. Run <code>docent validate</code> for the
    details.
  </p>
{:else if visual.type === 'sequence'}
  <Sequence {visual} />
{:else if visual.type === 'flow'}
  <Flow {visual} />
{:else if visual.type === 'state'}
  <State {visual} />
{:else}
  <Timeline {visual} />
{/if}

<style>
  .broken {
    margin: 0 0 1em;
    padding: 8px 11px;
    border-radius: 8px;
    background: var(--amber-soft);
    color: var(--amber);
    font-size: 13px;
  }

  code {
    font-family: var(--mono);
    font-size: 12px;
  }
</style>
