<script lang="ts">
  import type { RepoState } from '../lib/api';
  import { session } from '../lib/session.svelte';
  import { ago } from '../lib/status';

  // A fetch older than this is worth mentioning: a tracked branch is only as
  // fresh as the last fetch.
  const STALE_FETCH_DAYS = 7;

  interface Line {
    repo: RepoState;
    text: string;
    tone: 'amber' | 'red';
    action: 'fetch' | 'clone' | '';
  }

  /** What the reader is being asked to confirm, as "fetch api", or ''. */
  let asking = $state('');

  const repos = $derived(session.view?.repos ?? []);

  function counts(repo: string): string {
    const tally = new Map<string, number>();
    const steps = session.view?.tour?.steps ?? [];
    (session.view?.steps ?? []).forEach((summary, i) => {
      if (steps[i]?.repo === repo && summary.status !== 'ok') {
        tally.set(summary.status, (tally.get(summary.status) ?? 0) + 1);
      }
    });
    return [...tally].map(([status, n]) => `${n} ${status}`).join(' · ');
  }

  function describe(repo: RepoState): Line | null {
    if (!repo.found) {
      return { repo, text: `${repo.name} isn't checked out on this machine`, tone: 'red', action: 'clone' };
    }

    const parts: string[] = [];
    let action: Line['action'] = '';

    if (repo.ref?.problem) {
      parts.push(repo.ref.detail);
      action = 'fetch';
    }

    if (repo.stampUnreachable) {
      parts.push("the commit this tour was written against isn't in the local repository");
      action = 'fetch';
    } else if (repo.stamp && repo.behind > 0) {
      const age = ago(repo.stampedAt);
      const commits = `${repo.behind} ${repo.behind === 1 ? 'commit' : 'commits'}`;
      parts.push(
        `written against ${repo.stamp.slice(0, 7)}, ${commits}${age ? ` / ${age}` : ''} behind ${repo.ref?.name ?? 'the ref'}`,
      );
    }

    const drift = counts(repo.name);
    if (drift) {
      parts.push(drift);
    }

    if (repo.ref?.kind === 'tracking') {
      const fetched = ago(repo.lastFetch);
      const days = (Date.now() - new Date(repo.lastFetch).getTime()) / 86_400_000;
      if (fetched && days > STALE_FETCH_DAYS) {
        parts.push(`last fetched ${fetched} ago`);
        action = 'fetch';
      }
    }

    if (parts.length === 0) {
      return null;
    }
    return { repo, text: `${repo.name}: ${parts.join(' · ')}`, tone: 'amber', action };
  }

  const lines = $derived(repos.map(describe).filter((line): line is Line => line !== null));

  function command(line: Line): string {
    return line.action === 'clone' ? (line.repo.cloneCommand ?? []).join(' ') : `git -C ${line.repo.root} fetch origin`;
  }

  function confirm(line: Line): void {
    asking = '';
    void session.git(line.action as 'fetch' | 'clone', line.repo.name);
  }
</script>

{#if lines.length > 0 && !session.view?.workingTree}
  <ul class="freshness" aria-label="How current this tour is">
    {#each lines as line (line.repo.name)}
      {@const key = `${line.action} ${line.repo.name}`}
      <li class={line.tone}>
        <span class="text">{line.text}</span>
        {#if line.action && asking !== key}
          <button disabled={session.gitBusy !== ''} onclick={() => (asking = key)}>
            {session.gitBusy === key ? 'Working…' : line.action === 'clone' ? 'Clone…' : 'Fetch…'}
          </button>
        {/if}
        {#if asking === key}
          <!-- Docent runs git only when asked, and shows exactly what it will run. -->
          <span class="confirm" role="group" aria-label={`Confirm ${line.action}`}>
            <code class="selectable">{command(line)}</code>
            <button class="primary" onclick={() => confirm(line)}>Run it</button>
            <button onclick={() => session.copy(command(line), 'the command')}>Copy</button>
            <button onclick={() => (asking = '')}>Cancel</button>
          </span>
        {/if}
      </li>
    {/each}
  </ul>
{/if}

<style>
  .freshness {
    flex: none;
    list-style: none;
    margin: 0;
    padding: 6px 14px;
    display: flex;
    flex-direction: column;
    gap: 4px;
    border-bottom: 1px solid var(--border);
    font-size: 12.5px;
  }

  li {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
  }

  li::before {
    content: '';
    flex: none;
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--amber);
  }

  li.red::before {
    background: var(--red);
  }

  .text {
    color: var(--muted);
  }

  button {
    padding: 1px 8px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--raised);
    font-size: 12px;
  }

  button:hover:not(:disabled) {
    border-color: var(--accent);
  }

  button.primary {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--bg);
    font-weight: 600;
  }

  .confirm {
    display: inline-flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px;
  }

  code {
    font-family: var(--mono);
    font-size: 11.5px;
    padding: 2px 7px;
    border-radius: 5px;
    background: var(--hover);
    overflow-wrap: anywhere;
  }
</style>
