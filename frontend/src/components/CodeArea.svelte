<script lang="ts">
  import { api, errorMessage } from '../lib/api';
  import type { Editor, Entry } from '../lib/api';
  import { session } from '../lib/session.svelte';
  import { statusLabel, toneOf } from '../lib/status';
  import { basename } from '../lib/steps';
  import CodePane from './CodePane.svelte';
  import DirectoryPane from './DirectoryPane.svelte';

  // The step view lags the index by one round trip; only show it once it matches.
  const step = $derived(session.step?.index === session.index ? session.step : null);
  const extras = $derived(step?.anchors ?? []);

  /**
   * What the pane is showing: the step's own code, or one of its extra
   * anchors. Both have the same shape, so everything below reads `shown`.
   */
  const shown = $derived(session.tab >= 0 ? (extras[session.tab] ?? null) : step);
  const isFile = $derived(session.tab >= 0 || step?.kind === 'file');

  const then = $derived(shown?.then ?? null);
  const thenText = $derived((then?.lines ?? []).join('\n'));
  const stamp = $derived(session.view?.repos?.find((r) => r.name === shown?.repo)?.stamp.slice(0, 7) ?? 'the stamp');
  const range = $derived(shown?.location.range ?? { startLine: 0, startCol: 0, endLine: 0, endCol: 0 });

  /** Whether the code as it was is shown beside the code as it is. */
  let comparing = $state(false);

  // --- Open in… ---

  let editors = $state<Editor[]>([]);

  // Installed editors are looked up each time the menu opens, so one
  // installed or removed since is noticed.
  $effect(() => {
    if (session.editorMenu) {
      api
        .editors()
        .then((found) => (editors = found))
        .catch((err: unknown) => (session.error = errorMessage(err)));
    }
  });

  function rule(editor: Editor): string {
    const name = basename(shown?.path ?? '');
    const ext = name.includes('.') ? name.slice(name.lastIndexOf('.') + 1) : '';
    const match = ext ? { match: `**/*.${ext}` } : { repo: shown?.repo ?? '' };
    return JSON.stringify({ ...match, use: editor.id });
  }

  // --- The file drawer ---

  /** Directory listings fetched so far, by path. '' is the root. */
  let listings = $state<Record<string, Entry[]>>({});
  let expanded = $state<Record<string, boolean>>({});

  // A different step, tab or reload means a different tree.
  $effect(() => {
    void session.index;
    void session.tab;
    void session.path;
    void session.step;
    listings = {};
    expanded = {};
  });

  $effect(() => {
    if (session.drawer && isFile && shown && !listings['']) {
      void list('');
    }
  });

  async function list(dir: string): Promise<void> {
    try {
      const entries = await api.browse(session.path, session.index, session.tab, dir);
      listings = { ...listings, [dir]: entries };
    } catch (err) {
      session.error = errorMessage(err);
    }
  }

  function toggle(dir: string): void {
    expanded = { ...expanded, [dir]: !expanded[dir] };
    if (expanded[dir] && !listings[dir]) {
      void list(dir);
    }
  }
</script>

{#snippet tree(dir: string, depth: number)}
  <ul class="tree" role={depth === 0 ? 'tree' : 'group'}>
    {#each listings[dir] ?? [] as entry (entry.name)}
      {@const path = dir ? `${dir}/${entry.name}` : entry.name}
      <li
        role="treeitem"
        aria-selected={session.browsing?.path === path}
        aria-expanded={entry.isDir ? !!expanded[path] : undefined}
      >
        {#if entry.isDir}
          <button class="entry dir" style:padding-left={`${8 + depth * 12}px`} onclick={() => toggle(path)}>
            <span class="twist">{expanded[path] ? '▾' : '▸'}</span>{entry.name}
          </button>
          {#if expanded[path]}
            {@render tree(path, depth + 1)}
          {/if}
        {:else}
          <button
            class="entry"
            class:on={session.browsing?.path === path}
            style:padding-left={`${20 + depth * 12}px`}
            onclick={() => session.browse(path)}
          >
            {entry.name}
          </button>
        {/if}
      </li>
    {/each}
  </ul>
{/snippet}

<section class="pane">
  {#if extras.length > 0}
    <div class="tabs" role="tablist" aria-label="Code for this step">
      <button role="tab" aria-selected={session.tab === -1} onclick={() => session.showTab(-1)}>
        {step?.path ? basename(step.path) : 'Step'}
      </button>
      {#each extras as extra, i (i)}
        <button role="tab" aria-selected={session.tab === i} onclick={() => session.showTab(i)}>
          {extra.label}
          {#if toneOf(extra.location.status) !== 'ok'}
            <span class="dot {toneOf(extra.location.status)}"></span>
          {/if}
        </button>
      {/each}
    </div>
  {/if}

  <div class="crumbs">
    {#if session.browsing}
      <span class="trail">
        <span class="crumb root">{shown?.repo || session.workspace?.name}</span>
        {#each session.browsing.path.split('/').filter(Boolean) as part, i (i)}
          <span class="sep">›</span> <span class="crumb">{part}</span>
        {/each}
      </span>
      <span class="spacer"></span>
      <button class="open" onclick={() => session.browse('')}>Back to the step</button>
    {:else}
      <span class="trail">
        <span class="crumb root">{shown?.repo || session.workspace?.name}</span>
        {#each (shown?.path ?? '').split('/').filter(Boolean) as part, i (i)}
          <span class="sep">›</span> <span class="crumb">{part}</span>
        {/each}
      </span>
      {#if shown && range.startLine > 0}
        <span class="sep">›</span>
        <span class="crumb line">
          {range.startLine === range.endLine ? `line ${range.startLine}` : `lines ${range.startLine}–${range.endLine}`}
        </span>
      {/if}
      {#if shown?.ref?.commit}
        <span class="ref" title={shown.ref.detail || `Reading ${shown.ref.name} at ${shown.ref.commit}`}>
          {shown.ref.name} @ {shown.ref.commit.slice(0, 7)}
        </span>
      {:else if shown?.ref?.requested && session.view?.workingTree}
        <span class="ref" title="Reading files as they are on disk">working tree</span>
      {/if}
      <span class="spacer"></span>
      {#if then && shown?.content}
        <button class="open" aria-pressed={comparing} onclick={() => (comparing = !comparing)}>Then / Now</button>
      {/if}
      {#if shown?.permalink}
        <button class="open" onclick={() => session.copy(shown.permalink, 'the permalink')}>Copy permalink</button>
      {/if}
      {#if shown && statusLabel(shown.location)}
        <span class="badge {toneOf(shown.location.status)}" title={shown.location.detail}>
          {statusLabel(shown.location)}
        </span>
      {/if}
    {/if}

    {#if isFile && shown?.ref}
      <button class="open" aria-pressed={session.drawer} onclick={() => (session.drawer = !session.drawer)}
        >Files</button
      >
    {/if}
    {#if isFile && shown?.content && !session.browsing}
      <span class="split">
        <button class="open" onclick={() => session.openInEditor()} title="Open in your editor (o)"
          >Open in editor</button
        >
        <button
          class="open caret"
          aria-label="Open in…"
          aria-expanded={session.editorMenu}
          onclick={() => (session.editorMenu = !session.editorMenu)}
          title="Choose an editor for this step (⇧O)"
        >
          ▾
        </button>
        {#if session.editorMenu}
          <div class="menu" role="menu" aria-label="Open in…">
            {#each editors as editor (editor.id)}
              <div class="item">
                <button
                  role="menuitem"
                  onclick={() => {
                    session.editorMenu = false;
                    void session.openInEditor(editor.id);
                  }}
                >
                  {editor.name}
                </button>
                <button
                  class="rule"
                  title="Copy a rule for your config that always opens files like this one here"
                  onclick={() => session.copy(rule(editor), 'the rule')}
                >
                  Copy rule
                </button>
              </div>
            {:else}
              <p class="none">No editors were found.</p>
            {/each}
          </div>
        {/if}
      </span>
    {/if}
  </div>

  <div class="body">
    {#if session.drawer && isFile && shown?.ref}
      <nav class="drawer" aria-label="Files">
        {@render tree('', 0)}
      </nav>
    {/if}

    <div class="content">
      {#if session.browsing}
        {#if session.browsing.unavailable}
          <div class="placeholder">{session.browsing.unavailable}</div>
        {:else}
          <CodePane
            path={session.browsing.path}
            content={session.browsing.content}
            range={{ startLine: 0, startCol: 0, endLine: 0, endCol: 0 }}
          />
        {/if}
      {:else if !step || !shown}
        <div class="placeholder"></div>
      {:else if !isFile}
        {#if step.location.problem}
          <div class="placeholder">{step.location.detail}</div>
        {:else}
          <DirectoryPane path={step.path} entries={step.entries ?? []} readme={step.readme} />
        {/if}
      {:else if shown.unavailable}
        <div class="placeholder">{shown.unavailable}</div>
      {:else if shown.content === '' && shown.location.status === 'missing' && !then}
        <div class="placeholder">{shown.location.detail}</div>
      {:else if then && (comparing || shown.location.status === 'missing')}
        <!-- What the step was written about, beside what is there now. -->
        <div class="compare" class:alone={shown.location.status === 'missing'}>
          <div class="side">
            <p class="caption">Then, at {stamp}</p>
            <div class="code">
              <CodePane
                path={shown.path}
                content={thenText}
                range={{ startLine: 0, startCol: 0, endLine: 0, endCol: 0 }}
                firstLine={then.startLine}
              />
            </div>
          </div>
          {#if shown.location.status !== 'missing'}
            <div class="side">
              <p class="caption">Now</p>
              <div class="code">
                <CodePane path={shown.path} content={shown.content} {range} />
              </div>
            </div>
          {/if}
        </div>
      {:else}
        <CodePane path={shown.path} content={shown.content} {range} />
      {/if}
    </div>
  </div>
</section>

<style>
  .pane {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    border-right: 1px solid var(--border);
  }

  .tabs {
    flex: none;
    display: flex;
    gap: 2px;
    padding: 6px 10px 0;
    border-bottom: 1px solid var(--border);
    background: var(--panel);
    overflow-x: auto;
  }

  .tabs button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 11px;
    border: 1px solid transparent;
    border-bottom: none;
    border-radius: 7px 7px 0 0;
    font-size: 12.5px;
    color: var(--muted);
    white-space: nowrap;
  }

  .tabs button[aria-selected='true'] {
    background: var(--bg);
    border-color: var(--border);
    color: var(--text);
    font-weight: 600;
    margin-bottom: -1px;
  }

  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--amber);
  }

  .dot.red {
    background: var(--red);
  }

  .crumbs {
    flex: none;
    display: flex;
    align-items: center;
    gap: 6px;
    height: 34px;
    padding: 0 14px;
    border-bottom: 1px solid var(--border);
    font-family: var(--mono);
    font-size: 12px;
    color: var(--muted);
    white-space: nowrap;
  }

  .trail {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .crumbs > :not(.trail):not(.spacer) {
    flex: none;
  }

  .spacer {
    flex: 1;
  }

  .crumb.root {
    font-weight: 600;
    color: var(--text);
  }

  .crumb.line {
    color: var(--accent);
  }

  .sep {
    color: var(--faint);
  }

  .ref {
    margin-left: 6px;
    padding: 0 7px;
    border-radius: 999px;
    background: var(--hover);
    font-size: 11px;
    color: var(--muted);
  }

  .badge {
    font-family: var(--sans);
    font-size: 11px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--amber-soft);
    color: var(--amber);
  }

  .badge.red {
    background: var(--red-soft);
    color: var(--red);
  }

  .open {
    font-family: var(--sans);
    font-size: 12px;
    padding: 2px 9px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--raised);
    color: var(--text);
  }

  .open:hover {
    border-color: var(--accent);
  }

  .open[aria-pressed='true'] {
    background: var(--accent-soft);
    border-color: var(--accent);
  }

  .split {
    position: relative;
    display: inline-flex;
  }

  .split .open:first-child {
    border-radius: 6px 0 0 6px;
  }

  .caret {
    border-radius: 0 6px 6px 0;
    border-left: none;
    padding: 2px 6px;
  }

  .menu {
    position: absolute;
    z-index: 5;
    top: calc(100% + 4px);
    right: 0;
    min-width: 230px;
    padding: 4px;
    border: 1px solid var(--border);
    border-radius: 9px;
    background: var(--raised);
    box-shadow: var(--shadow);
    font-family: var(--sans);
  }

  .item {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .item button {
    padding: 5px 9px;
    border-radius: 6px;
    font-size: 13px;
    color: var(--text);
    text-align: left;
  }

  .item button:first-child {
    flex: 1;
  }

  .item button:hover {
    background: var(--hover);
  }

  .item .rule {
    font-size: 11.5px;
    color: var(--muted);
  }

  .none {
    margin: 0;
    padding: 6px 9px;
    font-size: 13px;
    color: var(--muted);
  }

  .body {
    flex: 1;
    min-height: 0;
    display: flex;
  }

  .drawer {
    flex: none;
    width: 230px;
    overflow: auto;
    border-right: 1px solid var(--border);
    background: var(--panel);
    padding: 6px 0;
  }

  .tree {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .entry {
    width: 100%;
    text-align: left;
    padding: 2px 8px;
    font-family: var(--mono);
    font-size: 12px;
    color: var(--muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .entry:hover {
    background: var(--hover);
  }

  .entry.dir {
    color: var(--text);
  }

  .entry.on {
    background: var(--accent-soft);
    color: var(--text);
  }

  .twist {
    display: inline-block;
    width: 12px;
    color: var(--faint);
  }

  .content {
    flex: 1;
    min-width: 0;
    min-height: 0;
  }

  .placeholder {
    height: 100%;
    display: grid;
    place-items: center;
    padding: 24px;
    color: var(--muted);
    text-align: center;
  }

  .compare {
    height: 100%;
    display: grid;
    grid-template-columns: 1fr 1fr;
  }

  .compare.alone {
    grid-template-columns: 1fr;
  }

  .side {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }

  .side + .side {
    border-left: 1px solid var(--border);
  }

  .caption {
    flex: none;
    margin: 0;
    padding: 4px 14px;
    font-family: var(--sans);
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--faint);
    border-bottom: 1px solid var(--border);
    background: var(--panel);
  }

  .side .code {
    flex: 1;
    min-height: 0;
  }
</style>
