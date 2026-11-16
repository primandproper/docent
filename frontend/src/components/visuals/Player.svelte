<script lang="ts">
  import type { Snippet } from 'svelte';
  import { session } from '../../lib/session.svelte';

  let {
    count,
    frame = $bindable(0),
    title = '',
    caption = '',
    step = '',
    kind,
    children,
  }: {
    /** How many frames there are. */
    count: number;
    frame?: number;
    title?: string;
    /** What the current frame shows, in words. */
    caption?: string;
    /** The id of the tour step the current frame belongs to, if any. */
    step?: string;
    kind: string;
    children: Snippet;
  } = $props();

  // How long each frame is held while playing.
  const HOLD_MS = 1500;

  let playing = $state(false);

  const stepIndex = $derived(step ? (session.view?.tour?.steps ?? []).findIndex((s) => s.id === step) : -1);

  $effect(() => {
    if (!playing) {
      return;
    }
    const timer = setInterval(() => {
      if (frame >= count - 1) {
        playing = false;
      } else {
        frame += 1;
      }
    }, HOLD_MS);
    return () => clearInterval(timer);
  });

  function toggle(): void {
    if (!playing && frame >= count - 1) {
      frame = 0;
    }
    playing = !playing;
  }

  function move(by: number): void {
    playing = false;
    frame = Math.max(0, Math.min(count - 1, frame + by));
  }
</script>

<figure class="visual" data-visual={kind}>
  {#if title}
    <figcaption class="title">{title}</figcaption>
  {/if}

  <div class="stage">{@render children()}</div>

  <p class="caption" aria-live="polite">
    <span>{caption}</span>
    {#if stepIndex >= 0 && stepIndex !== session.index}
      <button class="jump" onclick={() => session.go(stepIndex)}>Go to step {stepIndex + 1}</button>
    {/if}
  </p>

  <div class="controls" role="group" aria-label="Playback">
    <button onclick={() => move(-1)} disabled={frame === 0} aria-label="Previous frame">‹</button>
    <button class="play" onclick={toggle} aria-label={playing ? 'Pause' : 'Play'}>{playing ? 'Pause' : 'Play'}</button>
    <button onclick={() => move(1)} disabled={frame >= count - 1} aria-label="Next frame">›</button>
    <input
      type="range"
      min="0"
      max={Math.max(0, count - 1)}
      step="1"
      value={frame}
      oninput={(e) => {
        playing = false;
        frame = Number(e.currentTarget.value);
      }}
      aria-label="Scrub"
    />
    <span class="count">{frame + 1} / {count}</span>
  </div>
</figure>

<style>
  .visual {
    margin: 0 0 1em;
    padding: 12px;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--panel);
    /* The colors the visuals draw with, in one place. */
    --ink: var(--text);
    --line: var(--faint);
    --hot: var(--accent);
    --hot-soft: var(--accent-soft);
    --surface: var(--raised);
    --ease: 0.35s ease;
  }

  .title {
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: var(--muted);
    margin-bottom: 8px;
  }

  .stage {
    overflow-x: auto;
  }

  .stage :global(svg) {
    display: block;
    max-width: 100%;
    height: auto;
    margin: 0 auto;
    font-family: var(--sans);
  }

  .caption {
    min-height: 1.5em;
    margin: 8px 0 8px;
    font-size: 13px;
    display: flex;
    align-items: baseline;
    gap: 10px;
  }

  .jump {
    font-size: 12px;
    color: var(--accent);
    text-decoration: underline;
    text-underline-offset: 2px;
    white-space: nowrap;
  }

  .controls {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .controls button {
    min-width: 28px;
    padding: 2px 8px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--raised);
    font-size: 13px;
  }

  .controls button:disabled {
    opacity: 0.4;
  }

  .controls .play {
    min-width: 58px;
    font-weight: 600;
  }

  .controls input {
    flex: 1;
    min-width: 60px;
    accent-color: var(--accent);
  }

  .count {
    font-size: 12px;
    font-variant-numeric: tabular-nums;
    color: var(--muted);
    white-space: nowrap;
  }

  /* Honor a request for less motion: frames change at once, with no tweening. */
  @media (prefers-reduced-motion: reduce) {
    .visual {
      --ease: 0s;
    }
  }
</style>
