<script lang="ts">
  import { onMount } from 'svelte';
  import GoToStep from './components/GoToStep.svelte';
  import TourPicker from './components/TourPicker.svelte';
  import WalkView from './components/WalkView.svelte';
  import { session } from './lib/session.svelte';

  let goingTo = $state(false);
  let filterInput: HTMLInputElement | undefined = $state();

  onMount(() => {
    void session.init();
  });

  function isTyping(target: EventTarget | null): boolean {
    return target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement;
  }

  function onkeydown(event: KeyboardEvent): void {
    if (event.metaKey && !event.shiftKey && !event.altKey && !event.ctrlKey) {
      if (event.key === 'r') {
        void session.reload();
      } else if (event.key === 'o') {
        void session.chooseFolder();
      } else if (event.key === '[') {
        void session.back();
      } else if (event.key === ']') {
        void session.forward();
      } else {
        return;
      }
      event.preventDefault();
      return;
    }

    if (event.metaKey || event.ctrlKey || event.altKey || isTyping(event.target) || goingTo) {
      return;
    }

    const walking = session.view !== null;

    if (event.key === '/') {
      if (walking) {
        void session.close().then(() => filterInput?.focus());
      } else {
        filterInput?.focus();
      }
    } else if (!walking) {
      return;
    } else if (event.key === 'j' || event.key === 'ArrowDown' || event.key === 'ArrowRight') {
      void session.next();
    } else if (event.key === 'k' || event.key === 'ArrowUp' || event.key === 'ArrowLeft') {
      void session.previous();
    } else if (event.key === 'm') {
      session.mapShown = !session.mapShown;
    } else if (event.key === 'O') {
      session.editorMenu = !session.editorMenu;
    } else if (event.key === 'o') {
      void session.openInEditor();
    } else if (event.key === 'g') {
      goingTo = true;
    } else if (event.key === 'Escape' && session.editorMenu) {
      session.editorMenu = false;
    } else if (event.key === 'Escape') {
      void session.close();
    } else {
      return;
    }
    event.preventDefault();
  }
</script>

<svelte:window {onkeydown} />

{#if session.error}
  <div class="error selectable" role="alert">
    <span>{session.error}</span>
    <button onclick={() => (session.error = '')} aria-label="Dismiss">×</button>
  </div>
{/if}

{#if session.notice && !session.error}
  <div class="toast" role="status">{session.notice}</div>
{/if}

{#if session.loading}
  <div class="loading" aria-busy="true"></div>
{:else if session.view}
  <WalkView />
{:else}
  <TourPicker bind:filterInput />
{/if}

{#if goingTo}
  <GoToStep onclose={() => (goingTo = false)} />
{/if}

<style>
  .loading {
    height: 100%;
  }

  .error {
    position: fixed;
    z-index: 10;
    bottom: 16px;
    left: 50%;
    transform: translateX(-50%);
    max-width: min(720px, calc(100vw - 32px));
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 10px 12px 10px 14px;
    border-radius: 10px;
    border: 1px solid var(--red);
    background: var(--raised);
    color: var(--red);
    box-shadow: var(--shadow);
    font-size: 13px;
  }

  .toast {
    position: fixed;
    z-index: 10;
    bottom: 16px;
    left: 50%;
    transform: translateX(-50%);
    padding: 8px 14px;
    border-radius: 10px;
    border: 1px solid var(--border);
    background: var(--raised);
    box-shadow: var(--shadow);
    font-size: 13px;
  }

  .error button {
    font-size: 16px;
    line-height: 1;
    padding: 2px 4px;
    color: var(--muted);
  }
</style>
