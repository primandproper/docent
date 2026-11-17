<script lang="ts">
  import { untrack } from 'svelte';
  import type { Range } from '../lib/api';
  import { tokenize } from '../lib/highlight';
  import type { Token } from '../lib/highlight';

  let {
    path,
    content,
    range,
    firstLine = 1,
  }: {
    path: string;
    content: string;
    range: Range;
    /** The number shown beside the first line, for an excerpt that starts partway into a file. */
    firstLine?: number;
  } = $props();

  // Rows are a fixed height so the pane can render only the ones on screen: a
  // 20,000-line file costs the same to scroll as a 20-line one.
  const LINE_HEIGHT = 20;
  const OVERSCAN = 40;
  // Files up to this many lines are tokenized whole. Longer ones are tokenized
  // in a window around the viewport, which can miscolor the first lines of a
  // window that starts inside a multi-line construct.
  const WHOLE_FILE_LIMIT = 4000;
  const WINDOW_RADIUS = 1500;

  let scroller: HTMLDivElement | undefined = $state();
  let scrollTop = $state(0);
  let viewportHeight = $state(600);

  const lines = $derived(content.replace(/\n$/, '').split('\n'));
  const first = $derived(Math.max(0, Math.floor(scrollTop / LINE_HEIGHT) - OVERSCAN));
  const last = $derived(Math.min(lines.length, Math.ceil((scrollTop + viewportHeight) / LINE_HEIGHT) + OVERSCAN));
  const gutterWidth = $derived(`${String(lines.length + firstLine - 1).length + 2}ch`);

  /** Tokens for lines [tokenStart, tokenStart + tokens.length). */
  let tokens = $state.raw<Token[][]>([]);
  let tokenStart = $state(0);
  let tokenJob = 0;

  async function highlight(from: number, to: number): Promise<void> {
    const job = ++tokenJob;
    const result = await tokenize(path, lines.slice(from, to));
    if (job === tokenJob && result) {
      tokens = result;
      tokenStart = from;
    }
  }

  // A new file: drop the old colors, jump to the anchor, tokenize.
  $effect(() => {
    void content;
    void path;
    const start = range.startLine;

    untrack(() => {
      tokens = [];
      tokenStart = 0;

      const top = start > 0 ? Math.max(0, (start - 1) * LINE_HEIGHT - viewportHeight / 3) : 0;
      if (scroller) {
        scroller.scrollTop = top;
      }
      scrollTop = top;

      if (lines.length <= WHOLE_FILE_LIMIT) {
        void highlight(0, lines.length);
      } else {
        const center = Math.max(0, start - 1);
        void highlight(Math.max(0, center - WINDOW_RADIUS), Math.min(lines.length, center + WINDOW_RADIUS));
      }
    });
  });

  // In a long file, re-tokenize when scrolling leaves the tokenized window.
  $effect(() => {
    if (lines.length <= WHOLE_FILE_LIMIT) {
      return;
    }
    const from = first;
    const to = last;
    untrack(() => {
      if (tokens.length > 0 && (from < tokenStart || to > tokenStart + tokens.length)) {
        const center = Math.floor((from + to) / 2);
        void highlight(Math.max(0, center - WINDOW_RADIUS), Math.min(lines.length, center + WINDOW_RADIUS));
      }
    });
  });

  function inBand(line: number): boolean {
    return range.startLine > 0 && line >= range.startLine && line <= range.endLine;
  }
</script>

<div
  class="code selectable"
  bind:this={scroller}
  bind:clientHeight={viewportHeight}
  onscroll={() => (scrollTop = scroller?.scrollTop ?? 0)}
  style:--gutter={gutterWidth}
  style:--line={`${LINE_HEIGHT}px`}
  role="region"
  aria-label={path}
  tabindex="-1"
>
  <div class="sizer" style:height={`${lines.length * LINE_HEIGHT}px`}>
    <div class="window" style:transform={`translateY(${first * LINE_HEIGHT}px)`}>
      {#each { length: last - first } as _, offset (first + offset)}
        {@const n = first + offset + 1}
        {@const row = tokens[n - 1 - tokenStart]}
        <div
          class="row"
          data-line={n}
          class:band={inBand(n)}
          class:band-start={n === range.startLine}
          class:band-end={range.startLine > 0 && n === range.endLine}
        >
          <span class="gutter">{n + firstLine - 1}</span>
          <span class="text"
            >{#if row}{#each row as token, t (t)}<span
                  style:--l={token.light}
                  style:--d={token.dark}
                  class:italic={token.italic}
                  class:bold={token.bold}>{token.text}</span
                >{/each}{:else}{lines[n - 1]}{/if}</span
          >
        </div>
      {/each}
    </div>
  </div>
</div>

<style>
  .code {
    height: 100%;
    overflow: auto;
    font-family: var(--mono);
    font-size: 12.5px;
    line-height: var(--line);
    tab-size: 4;
    background: var(--bg);
    overflow-anchor: none;
  }

  .sizer {
    position: relative;
    min-width: max-content;
  }

  .window {
    min-width: max-content;
    will-change: transform;
  }

  .row {
    display: flex;
    height: var(--line);
    min-width: 100%;
  }

  .gutter {
    position: sticky;
    left: 0;
    flex: none;
    width: var(--gutter);
    padding-right: 1ch;
    text-align: right;
    color: var(--faint);
    background: var(--bg);
    border-left: 3px solid transparent;
    user-select: none;
    -webkit-user-select: none;
    font-variant-numeric: tabular-nums;
  }

  .text {
    white-space: pre;
    padding: 0 16px 0 12px;
    flex: 1;
  }

  .text span {
    color: var(--l);
  }

  @media (prefers-color-scheme: dark) {
    .text span {
      color: var(--d);
    }
  }

  .italic {
    font-style: italic;
  }

  .bold {
    font-weight: 600;
  }

  .band {
    background: var(--band);
  }

  .band .gutter {
    border-left-color: var(--accent);
    color: var(--text);
    /* The gutter is opaque so text can scroll under it; repaint the band on top of it. */
    background: linear-gradient(var(--band), var(--band)), var(--bg);
  }
</style>
