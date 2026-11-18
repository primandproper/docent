<script lang="ts">
  import { session } from '../lib/session.svelte';
</script>

<!-- Back and forward through where the reader has been: steps jumped to, other tours, the picker. -->
<span class="history">
  <button disabled={!session.canBack} onclick={() => session.back()} title="Back (⌘[ or ⌘←)" aria-label="Back">←</button
  >
  <button
    disabled={!session.canForward}
    onclick={() => session.forward()}
    title="Forward (⌘] or ⌘→)"
    aria-label="Forward">→</button
  >
</span>

<style>
  .history {
    flex: none;
    display: inline-flex;
    gap: 2px;
  }

  button {
    width: 26px;
    height: 24px;
    border-radius: 6px;
    color: var(--muted);
    font-size: 14px;
    line-height: 1;
  }

  button:hover:not(:disabled) {
    background: var(--hover);
    color: var(--text);
  }

  button:disabled {
    color: var(--faint);
    opacity: 0.5;
    cursor: default;
  }
</style>
