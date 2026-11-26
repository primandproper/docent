<script lang="ts">
  import { api } from '../lib/api';
  import type { Card } from '../lib/api';
  import { parseInternal } from '../lib/internal';
  import { session } from '../lib/session.svelte';
  import FlagCard from './FlagCard.svelte';

  let { cards }: { cards: Card[] } = $props();

  function open(card: Card): void {
    if (parseInternal(card.url)) {
      void session.follow(card.url);
    } else {
      void api.openExternal(card.url);
    }
  }
</script>

{#if cards.length > 0}
  <ul class="links">
    {#each cards as card, i (i)}
      <li>
        {#if card.flag && session.view?.flagStatus}
          <FlagCard {card} />
        {:else}
          <!-- A card always shows where it goes before anyone clicks it. -->
          <button
            class="link-card"
            disabled={!!card.problem || !card.url}
            onclick={() => open(card)}
            title={card.problem || card.url}
          >
            <span class="kind">{card.kind}</span>
            <span class="title">{card.title}</span>
            {#if card.description}<span class="about">{card.description}</span>{/if}
            <!-- A flag card with nowhere to go still says which flag it means. -->
            <span class="where" class:plain={!card.problem}
              >{card.problem || card.host || card.flag || 'In this workspace'}</span
            >
          </button>
        {/if}
      </li>
    {/each}
  </ul>
{/if}

<style>
  .links {
    list-style: none;
    margin: 18px 0 1em;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .link-card {
    width: 100%;
    text-align: left;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 11px 13px;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--raised);
    font-family: var(--sans);
    font-size: 14px;
    line-height: 1.5;
    color: var(--text);
  }

  .link-card:hover:not(:disabled) {
    border-color: var(--accent);
  }

  .link-card:disabled {
    opacity: 0.6;
  }

  .kind {
    font-size: 10.5px;
    font-weight: 700;
    letter-spacing: 0.07em;
    text-transform: uppercase;
    color: var(--accent);
  }

  .title {
    font-weight: 600;
  }

  .about {
    color: var(--muted);
    font-size: 13px;
  }

  .where {
    font-family: var(--mono);
    font-size: 11.5px;
    color: var(--muted);
    overflow-wrap: anywhere;
  }

  .link-card:disabled .where {
    color: var(--amber);
  }

  .link-card:disabled .where.plain {
    color: var(--muted);
  }
</style>
