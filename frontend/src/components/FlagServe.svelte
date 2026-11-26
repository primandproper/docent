<!-- What a rule of a feature flag hands out: one variation, or a rollout drawn as a bar. -->
<script lang="ts">
  import { pretty, variationLabel, variationTone } from '../lib/flags';
  import type { Serve, Variation } from '../lib/flags';

  let {
    serve,
    variations,
    showValue = false,
    unfolded = false,
  }: {
    serve: Serve;
    variations: Variation[] | null;
    /** Show the variation's value beside its name, when it has one worth reading. */
    showValue?: boolean;
    /** Start with a JSON value laid out, not as a line to click. */
    unfolded?: boolean;
  } = $props();

  const shares = $derived(serve.rollout ?? []);
  const label = $derived(variationLabel(variations, serve.variation));
  const tone = $derived(variationTone(variations, serve.variation));

  const value = $derived.by(() => {
    const v = variations?.[serve.variation];
    if (!showValue || !v) {
      return null;
    }
    const p = pretty(v.value);
    // A named scalar still has a value to show; an unnamed one is already its own label.
    return p.block || (v.name && v.value !== 'true' && v.value !== 'false') ? p : null;
  });
</script>

{#if shares.length > 0}
  <span class="rollout">
    <span class="bar" aria-hidden="true">
      {#each shares as share (share.variation)}
        <span class="slice {variationTone(variations, share.variation)}" style:width="{share.percent}%"></span>
      {/each}
    </span>
    {#each shares as share (share.variation)}
      <span class="share"
        ><span class="swatch {variationTone(variations, share.variation)}"></span>{share.percent}%
        {variationLabel(variations, share.variation)}</span
      >
    {/each}
  </span>
{:else if value?.block}
  <details class="json" open={unfolded}>
    <summary>
      <span class="chip"><span class="swatch {tone}"></span>{label}</span>
      <code class="peek">{value.line}</code>
    </summary>
    <pre class="selectable">{value.text}</pre>
  </details>
{:else}
  <span class="chip"><span class="swatch {tone}"></span>{label}</span>
  {#if value}<code class="peek">{value.line}</code>{/if}
{/if}

<style>
  .chip,
  .share {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    white-space: nowrap;
  }

  .chip {
    padding: 0 7px;
    border-radius: 5px;
    border: 1px solid var(--border);
    background: var(--raised);
    font-weight: 600;
  }

  .share {
    margin-right: 10px;
  }

  .rollout {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 3px 0;
    width: 100%;
  }

  .bar {
    display: flex;
    width: 100%;
    height: 6px;
    border-radius: 3px;
    overflow: hidden;
    margin: 3px 0 2px;
    gap: 1px;
  }

  .slice,
  .swatch {
    background: var(--tone);
  }

  .swatch {
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 2px;
  }

  .json {
    flex: 1;
    min-width: 0;
  }

  .json > summary {
    display: flex;
    align-items: center;
    gap: 7px;
    list-style: none;
    cursor: default;
  }

  .json > summary::-webkit-details-marker {
    display: none;
  }

  .json > summary::after {
    content: '▸';
    color: var(--muted);
    font-size: 10px;
  }

  .json[open] > summary::after {
    content: '▾';
  }

  .json[open] .peek {
    visibility: hidden;
  }

  .peek {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font: 11px var(--mono);
    color: var(--muted);
  }

  pre {
    margin: 5px 0 2px;
    padding: 7px 9px;
    border-radius: 6px;
    background: var(--border);
    font: 11.5px/1.45 var(--mono);
    max-height: 190px;
    overflow: auto;
  }
</style>
