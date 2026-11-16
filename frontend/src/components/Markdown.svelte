<script lang="ts">
  import { api, isExternalURL } from '../lib/api';
  import { renderMarkdown } from '../lib/markdown';

  let { source }: { source: string } = $props();

  const html = $derived(renderMarkdown(source));

  // Links never navigate the webview. Web links open in the system browser;
  // anything else a tour links to (CodeTour's command: links, file paths) does nothing.
  function onclick(event: MouseEvent): void {
    const link = (event.target as Element | null)?.closest('a');
    if (!link) {
      return;
    }
    event.preventDefault();
    const href = link.getAttribute('href') ?? '';
    if (isExternalURL(href)) {
      void api.openExternal(href);
    }
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="markdown selectable" {onclick}>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- renderMarkdown sanitizes with DOMPurify -->
  {@html html}
</div>

<style>
  .markdown {
    line-height: 1.6;
    overflow-wrap: anywhere;
  }

  .markdown :global(:first-child) {
    margin-top: 0;
  }

  .markdown :global(:last-child) {
    margin-bottom: 0;
  }

  .markdown :global(h1),
  .markdown :global(h2),
  .markdown :global(h3),
  .markdown :global(h4) {
    line-height: 1.3;
    margin: 1.3em 0 0.5em;
    font-weight: 650;
  }

  .markdown :global(h1) {
    font-size: 1.35em;
  }

  .markdown :global(h2) {
    font-size: 1.2em;
  }

  .markdown :global(h3),
  .markdown :global(h4) {
    font-size: 1.05em;
  }

  .markdown :global(p),
  .markdown :global(ul),
  .markdown :global(ol),
  .markdown :global(pre),
  .markdown :global(blockquote),
  .markdown :global(table) {
    margin: 0 0 0.9em;
  }

  .markdown :global(ul),
  .markdown :global(ol) {
    padding-left: 1.4em;
  }

  .markdown :global(a) {
    color: var(--accent);
    text-decoration: underline;
    text-underline-offset: 2px;
    cursor: pointer;
  }

  .markdown :global(code) {
    font-family: var(--mono);
    font-size: 0.88em;
    background: var(--hover);
    padding: 0.1em 0.35em;
    border-radius: 4px;
  }

  .markdown :global(pre) {
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 10px 12px;
    overflow-x: auto;
    line-height: 1.5;
  }

  .markdown :global(pre code) {
    background: none;
    padding: 0;
    overflow-wrap: normal;
  }

  .markdown :global(blockquote) {
    margin-left: 0;
    padding: 2px 0 2px 12px;
    border-left: 3px solid var(--border);
    color: var(--muted);
  }

  .markdown :global(table) {
    border-collapse: collapse;
    display: block;
    overflow-x: auto;
  }

  .markdown :global(th),
  .markdown :global(td) {
    border: 1px solid var(--border);
    padding: 4px 9px;
    text-align: left;
  }

  .markdown :global(hr) {
    border: none;
    border-top: 1px solid var(--border);
    margin: 1.2em 0;
  }
</style>
