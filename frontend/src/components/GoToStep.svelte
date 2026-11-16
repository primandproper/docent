<script lang="ts">
  import { session } from '../lib/session.svelte';
  import { parseStepNumber } from '../lib/steps';

  let { onclose }: { onclose: () => void } = $props();

  let value = $state('');
  let input: HTMLInputElement | undefined = $state();

  const target = $derived(parseStepNumber(value, session.stepCount));

  $effect(() => {
    input?.focus();
  });

  function onkeydown(event: KeyboardEvent): void {
    if (event.key === 'Enter' && target !== null) {
      void session.go(target);
      onclose();
    } else if (event.key === 'Escape') {
      onclose();
    } else {
      return;
    }
    event.preventDefault();
  }
</script>

<div class="scrim" onclick={onclose} role="presentation"></div>
<div class="dialog" role="dialog" aria-label="Go to step">
  <label for="goto">Go to step</label>
  <input
    id="goto"
    bind:this={input}
    bind:value
    {onkeydown}
    inputmode="numeric"
    autocomplete="off"
    placeholder={`1–${session.stepCount}`}
    aria-invalid={value !== '' && target === null}
  />
</div>

<style>
  .scrim {
    position: fixed;
    inset: 0;
  }

  .dialog {
    position: fixed;
    top: 18%;
    left: 50%;
    transform: translateX(-50%);
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 14px;
    border: 1px solid var(--border);
    border-radius: 12px;
    background: var(--raised);
    box-shadow: var(--shadow);
  }

  label {
    font-weight: 600;
  }

  input {
    font: inherit;
    width: 90px;
    padding: 5px 9px;
    border-radius: 7px;
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
  }

  input[aria-invalid='true'] {
    border-color: var(--red);
  }
</style>
