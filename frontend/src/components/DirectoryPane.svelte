<script lang="ts">
  import type { Entry } from '../lib/api';
  import Markdown from './Markdown.svelte';

  let { path, entries, readme }: { path: string; entries: Entry[]; readme: string } = $props();
</script>

<div class="directory">
  <ul class="entries selectable" aria-label={`Contents of ${path}`}>
    {#each entries as entry (entry.name)}
      <li class:dir={entry.isDir}>{entry.name}{entry.isDir ? '/' : ''}</li>
    {/each}
  </ul>

  {#if readme}
    <section class="readme">
      <Markdown source={readme} />
    </section>
  {/if}
</div>

<style>
  .directory {
    height: 100%;
    overflow-y: auto;
    padding: 16px 24px 32px;
  }

  .entries {
    list-style: none;
    margin: 0;
    padding: 0;
    columns: 220px;
    column-gap: 24px;
    font-family: var(--mono);
    font-size: 12.5px;
    line-height: 1.8;
  }

  .entries li {
    color: var(--muted);
    break-inside: avoid;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .entries li.dir {
    color: var(--text);
    font-weight: 600;
  }

  .readme {
    margin-top: 20px;
    padding-top: 16px;
    border-top: 1px solid var(--border);
    max-width: 760px;
  }
</style>
