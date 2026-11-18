<script lang="ts">
  import type { TourSummary } from '../lib/api';
  import { groupByFolder } from '../lib/folders';
  import { session } from '../lib/session.svelte';
  import HistoryNav from './HistoryNav.svelte';

  let { filterInput = $bindable() }: { filterInput?: HTMLInputElement } = $props();

  let query = $state('');
  let selected = $state(0);

  const matches = $derived.by(() => {
    const q = query.trim().toLowerCase();
    if (!q) {
      return session.tours;
    }
    return session.tours.filter((t) =>
      [t.title, t.description, t.displayPath, ...(t.tags ?? [])].some((field) => field.toLowerCase().includes(q)),
    );
  });

  // Tours are shown under their folders. `matches` is in folder order too, which is what ↑ and ↓ walk.
  const groups = $derived(groupByFolder(session.folders, matches, query.trim() !== ''));
  const position = $derived(new Map(groups.flatMap((group) => group.tours).map((tour, i) => [tour.path, i])));
  const shown = $derived(groups.flatMap((group) => group.tours));

  $effect(() => {
    if (selected >= matches.length) {
      selected = Math.max(0, matches.length - 1);
    }
  });

  function health(tour: TourSummary): { text: string; tone: string } {
    if (tour.unreadable) {
      return { text: 'Not a valid tour', tone: 'red' };
    }
    if (tour.errors > 0) {
      return { text: `${tour.errors} ${tour.errors === 1 ? 'problem' : 'problems'}`, tone: 'red' };
    }
    if (tour.warnings > 0) {
      return { text: `${tour.warnings} drifted`, tone: 'amber' };
    }
    return { text: 'All anchors resolve', tone: 'ok' };
  }

  function open(tour: TourSummary | undefined): void {
    if (tour && !tour.unreadable) {
      void session.open(tour.path);
    }
  }

  function onkeydown(event: KeyboardEvent): void {
    if (event.key === 'ArrowDown') {
      selected = Math.min(selected + 1, matches.length - 1);
    } else if (event.key === 'ArrowUp') {
      selected = Math.max(selected - 1, 0);
    } else if (event.key === 'Enter') {
      open(shown[selected]);
    } else if (event.key === 'Escape') {
      query = '';
      filterInput?.blur();
    } else {
      return;
    }
    event.preventDefault();
  }
</script>

<main class="picker">
  <header>
    <div class="history"><HistoryNav /></div>
    <div class="heading">
      <h1>Docent</h1>
      {#if session.folders.length > 0}
        <span class="spacer"></span>
        <button class="add" onclick={() => session.addFolder()} title="Show another folder of tours beside these (⌘O)">
          Add a folder…
        </button>
      {/if}
    </div>
  </header>

  {#if session.folders.length === 0}
    <div class="empty">
      <h2>Add a folder to begin</h2>
      <p>Pick a folder that holds tours, or the folder your repos are cloned into. You can add more than one.</p>
      <p><button class="choose" onclick={() => session.addFolder()}>Add a folder…</button> <kbd>⌘O</kbd></p>
      <p>From a terminal, <code>docent .</code> opens the folder you are in.</p>
    </div>
  {:else}
    {#if session.tours.length > 0}
      <input
        bind:this={filterInput}
        bind:value={query}
        {onkeydown}
        class="filter"
        type="search"
        placeholder="Search tours"
        aria-label="Search tours"
        autocomplete="off"
        spellcheck="false"
      />
    {/if}

    <div class="folders">
      {#each groups as group (group.folder.root)}
        {@const folder = group.folder}
        <section class="folder" aria-label={folder.name}>
          <div class="folder-head">
            <h2>{folder.name}</h2>
            <span class="root selectable" title={folder.root}><bdi>{folder.root}</bdi></span>
            <span class="spacer"></span>
            {#if !folder.saved}
              <span class="unsaved" title="This folder was opened from outside the app, and is here until you quit.">
                Open for now
              </span>
              <button
                class="folder-action"
                onclick={() => session.keepFolder(folder.root)}
                title="Show this folder every time Docent opens"
                aria-label={`Keep ${folder.name}`}
              >
                Keep
              </button>
            {/if}
            <button
              class="folder-action"
              onclick={() => session.removeFolder(folder.root)}
              title="Stop showing this folder. Nothing on disk is deleted."
              aria-label={`Remove ${folder.name}`}
            >
              Remove
            </button>
          </div>

          {#if folder.problem}
            <p class="folder-note red">{folder.problem}</p>
          {:else if group.tours.length === 0}
            <p class="folder-note">
              No tours here yet. Docent looks for <code>.tour</code> files in <code>.tours/</code>,
              <code>.vscode/tours/</code> and <code>.github/tours/</code> under this folder. Press <kbd>⌘R</kbd> to look again.
            </p>
          {:else}
            <ul class="tours">
              {#each group.tours as tour (tour.path)}
                {@const h = health(tour)}
                {@const i = position.get(tour.path) ?? 0}
                <li>
                  <button
                    class="tour"
                    class:selected={i === selected}
                    disabled={tour.unreadable}
                    onclick={() => open(tour)}
                    onmousemove={() => (selected = i)}
                  >
                    <span class="line">
                      <span class="title">{tour.title}</span>
                      {#if tour.unreviewed}
                        <span class="badge amber">Unreviewed draft</span>
                      {/if}
                      <span class="spacer"></span>
                      <span class="health {h.tone}">{h.text}</span>
                    </span>
                    {#if tour.description}
                      <span class="description">{tour.description}</span>
                    {/if}
                    <span class="meta">
                      {tour.steps}
                      {tour.steps === 1 ? 'step' : 'steps'}
                      {#if (tour.repos ?? []).length > 0}· {(tour.repos ?? []).join(', ')}{/if}
                      · {tour.displayPath}
                      {#each tour.tags ?? [] as tag (tag)}
                        <span class="tag">{tag}</span>
                      {/each}
                    </span>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {:else}
        <p class="none">No tours match “{query}”.</p>
      {/each}
    </div>

    <footer>
      <kbd>/</kbd> search <kbd>↑</kbd><kbd>↓</kbd> choose <kbd>↵</kbd> open <kbd>⌘R</kbd> reload <kbd>⌘O</kbd> add a
      folder
      <kbd>⌘[</kbd><kbd>⌘]</kbd> back, forward
    </footer>
  {/if}
</main>

<style>
  .picker {
    height: 100%;
    max-width: 760px;
    margin: 0 auto;
    padding: 56px 24px 20px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .history {
    margin: 0 0 10px -6px;
  }

  h1 {
    margin: 0;
    font-size: 26px;
    font-weight: 650;
    letter-spacing: -0.01em;
  }

  .heading {
    display: flex;
    align-items: center;
  }

  .add,
  .folder-action {
    color: var(--muted);
    border: 1px solid var(--border);
    background: var(--raised);
    border-radius: 6px;
    white-space: nowrap;
  }

  .add {
    padding: 5px 11px;
    font-size: 13px;
  }

  .folder-action {
    padding: 2px 8px;
    font-size: 12px;
  }

  .add:hover,
  .folder-action:hover {
    background: var(--hover);
    color: var(--text);
  }

  .folders {
    overflow-y: auto;
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 22px;
  }

  .folder-head {
    display: flex;
    align-items: baseline;
    gap: 10px;
    margin-bottom: 8px;
  }

  .folder-head h2 {
    margin: 0;
    font-size: 15px;
    font-weight: 650;
    white-space: nowrap;
  }

  .root {
    color: var(--faint);
    font-family: var(--mono);
    font-size: 12px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    /* A long path loses its start, not the folder at its end. */
    direction: rtl;
    text-align: left;
  }

  .unsaved {
    color: var(--faint);
    font-size: 12px;
    white-space: nowrap;
  }

  .folder-note {
    margin: 0;
    padding: 12px 14px;
    border: 1px dashed var(--border);
    border-radius: 10px;
    color: var(--muted);
  }

  .folder-note.red {
    color: var(--red);
  }

  .filter {
    font: inherit;
    color: var(--text);
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 9px 12px;
    width: 100%;
  }

  .tours {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .tour {
    width: 100%;
    text-align: left;
    display: flex;
    flex-direction: column;
    gap: 3px;
    padding: 12px 14px;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--raised);
  }

  .tour.selected {
    border-color: var(--accent);
    background: var(--accent-soft);
  }

  .tour:disabled {
    opacity: 0.6;
  }

  .line {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .title {
    font-weight: 600;
    font-size: 15px;
  }

  .spacer {
    flex: 1;
  }

  .description {
    color: var(--muted);
  }

  .meta {
    color: var(--faint);
    font-size: 12px;
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }

  .tag,
  .badge {
    font-size: 11px;
    padding: 1px 7px;
    border-radius: 999px;
    background: var(--hover);
    color: var(--muted);
    white-space: nowrap;
  }

  .badge.amber {
    background: var(--amber-soft);
    color: var(--amber);
    font-weight: 600;
  }

  .health {
    font-size: 12px;
    white-space: nowrap;
  }

  .health.ok {
    color: var(--green);
  }

  .health.amber {
    color: var(--amber);
  }

  .health.red {
    color: var(--red);
  }

  .none,
  .empty {
    color: var(--muted);
  }

  .empty {
    margin-top: 32px;
    padding: 28px;
    border: 1px dashed var(--border);
    border-radius: 12px;
  }

  .empty h2 {
    margin: 0 0 8px;
    font-size: 17px;
    color: var(--text);
  }

  .choose {
    padding: 6px 13px;
    border-radius: 8px;
    background: var(--accent);
    color: var(--bg);
    font-weight: 600;
  }

  code {
    font-family: var(--mono);
    font-size: 12.5px;
  }

  footer {
    color: var(--faint);
    font-size: 12px;
    display: flex;
    gap: 6px;
    align-items: center;
  }

  footer kbd:not(:first-child) {
    margin-left: 10px;
  }

  footer kbd + kbd {
    margin-left: -3px;
  }
</style>
