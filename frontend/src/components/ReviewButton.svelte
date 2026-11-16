<script lang="ts">
  import { tick } from 'svelte';
  import { session } from '../lib/session.svelte';

  // Marking a tour reviewed is the one edit the app makes to a tour, so it
  // asks first, and Go only allows it once every step has been read.
  const review = $derived(session.review);
  const read = $derived((review?.read ?? []).filter(Boolean).length);
  const total = $derived((review?.read ?? []).length);

  let name = $state('');
  let input: HTMLInputElement | undefined = $state();

  function ask(): void {
    if (!review?.ready) {
      // Say why, where a disabled button would only sit there.
      session.say(review?.blocker ?? '');
      return;
    }
    name = review.name;
    session.reviewAsking = !session.reviewAsking;
    if (session.reviewAsking) {
      void tick().then(() => input?.select());
    }
  }

  async function sign(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (name.trim() === '' || session.reviewing) {
      return;
    }
    if (await session.markReviewed(name)) {
      session.reviewAsking = false;
    }
  }

  function onkeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      session.reviewAsking = false;
    }
  }
</script>

{#if review?.available}
  <span class="review">
    <button
      class="mark"
      class:ready={review.ready}
      aria-disabled={!review.ready}
      aria-expanded={session.reviewAsking}
      title={review.ready ? 'Vouch for this tour: you walked it and it is right' : review.blocker}
      onclick={ask}
    >
      Mark reviewed
      {#if read < total}<span class="count">{read}/{total} read</span>{/if}
    </button>
    {#if session.reviewAsking && review.ready}
      <form class="ask" aria-label="Mark reviewed" onsubmit={sign}>
        <p>
          This adds your name to the tour file and stamps it at the commits you just read. Commit the file to share the
          review.
        </p>
        <label>
          <span>Reviewed by</span>
          <input
            bind:this={input}
            bind:value={name}
            {onkeydown}
            maxlength="100"
            placeholder="Your name"
            spellcheck="false"
            autocomplete="off"
          />
        </label>
        <div class="actions">
          <button type="button" onclick={() => (session.reviewAsking = false)}>Cancel</button>
          <button type="submit" class="sign" disabled={name.trim() === '' || session.reviewing}>
            {session.reviewing ? 'Saving…' : 'Sign and save'}
          </button>
        </div>
      </form>
    {/if}
  </span>
{/if}

<style>
  .review {
    position: relative;
    flex: none;
  }

  .mark {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    font-size: 12px;
    padding: 2px 9px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--raised);
    color: var(--faint);
    white-space: nowrap;
    cursor: default;
  }

  .mark.ready {
    border-color: var(--green);
    background: var(--green-soft);
    color: var(--green);
    font-weight: 600;
    cursor: pointer;
  }

  .count {
    font-variant-numeric: tabular-nums;
    font-size: 11px;
  }

  .ask {
    position: absolute;
    z-index: 5;
    top: calc(100% + 6px);
    left: 0;
    width: 300px;
    padding: 12px;
    border: 1px solid var(--border);
    border-radius: 9px;
    background: var(--raised);
    box-shadow: var(--shadow);
    display: flex;
    flex-direction: column;
    gap: 10px;
    font-size: 12.5px;
  }

  .ask p {
    margin: 0;
    color: var(--muted);
    line-height: 1.45;
  }

  .ask label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    color: var(--muted);
    font-size: 11.5px;
  }

  .ask input {
    font: inherit;
    font-size: 13px;
    color: var(--text);
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 5px 8px;
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }

  .actions button {
    font-size: 12px;
    padding: 4px 11px;
    border-radius: 6px;
    border: 1px solid var(--border);
    color: var(--muted);
  }

  .actions .sign {
    border-color: var(--green);
    background: var(--green);
    color: var(--bg);
    font-weight: 600;
  }

  .actions .sign:disabled {
    opacity: 0.5;
  }
</style>
