<!--
  A `flag` link card with the flag's live status: how it decides, in the one environment the reader picked, in the
  order it is evaluated (prerequisites, named targets, rules, then everyone else). Variations and the other
  environments fold away underneath, so a flag with fourteen environments costs one line.

  Everything shown here came from the integration and is drawn as plain text.
-->
<script lang="ts">
  import { api } from '../lib/api';
  import type { Card } from '../lib/api';
  import {
    ahead,
    clauseWords,
    decisions,
    flagKind,
    only,
    pretty,
    segmentSize,
    variationLabel,
    variationTone,
  } from '../lib/flags';
  import type { Status } from '../lib/flags';
  import { session } from '../lib/session.svelte';
  import FlagServe from './FlagServe.svelte';

  let { card }: { card: Card } = $props();

  let status = $state<Status | null>(null);
  let allEnvs = $state(false);

  // Not reactive: only there to tell a reload the reader asked for from everything else.
  let askedAt = session.flagEpoch;

  // Asked again whenever the card is (a new step, a new choice of environment), and afresh on ⌘R.
  $effect(() => {
    const { flag, project } = card;
    const epoch = session.flagEpoch;
    const fresh = epoch !== askedAt;
    askedAt = epoch;

    let stale = false;
    status = null;
    api
      .flagStatus(session.path, flag, project, fresh)
      .then((answer) => {
        if (!stale) {
          status = answer;
        }
      })
      .catch(() => {
        if (!stale) {
          status = unknown('Docent could not ask');
        }
      });
    return () => {
      stale = true;
    };
  });

  function unknown(reason: string): Status {
    return {
      state: 'unavailable',
      reason,
      name: '',
      description: '',
      project: '',
      environment: '',
      url: '',
      kind: '',
      environments: [],
      variations: [],
      segments: [],
      targeting: null,
    };
  }

  const url = $derived(card.url || status?.url || '');
  const t = $derived(status?.targeting ?? null);
  const variations = $derived(status?.variations ?? []);
  const envs = $derived(status?.environments ?? []);
  const shown = $derived(envs.find((e) => e.shown));
  const envName = $derived(shown?.name ?? status?.environment ?? '');
  const pinned = $derived(envs.filter((e) => e.pinned && !e.shown));
  const rest = $derived(envs.filter((e) => !e.pinned && !e.shown));
  const on = $derived(status?.state === 'on');
  const archived = $derived(status?.state === 'archived');
  const before = $derived(status ? ahead(status) : '');
  const lines = $derived(status ? decisions(status) : 0);
  /** How many ways the default splits, when it is a rollout. */
  const split = $derived(t?.fallthrough.rollout?.length ?? 0);
  // With only a few outcomes there is room to lay each JSON value out; with many, each is a line to click.
  const roomy = $derived(lines <= 2);

  /** The segments a clause points at, as far as the integration described them. */
  function segmentsOf(keys: string[] | null) {
    return (keys ?? []).flatMap((key) => (status?.segments ?? []).filter((segment) => segment.key === key));
  }

  function open(): void {
    if (url) {
      void api.openExternal(url);
    }
  }
</script>

<div class="flag-card" data-state={status?.state ?? 'loading'}>
  <div class="head">
    <span class="title">{card.title}</span>
    {#if status && variations.length > 0}<span class="type">{flagKind(status)}</span>{/if}
    <button class="out" disabled={!url} onclick={open} title={url}>LaunchDarkly ↗</button>
  </div>
  <code class="key">{card.flag}</code>
  {#if card.description}<p class="about">{card.description}</p>{/if}

  {#if !status}
    <div class="panel ghost" role="status" aria-label="Checking the flag's status"></div>
  {:else if status.state === 'not-found'}
    <p class="banner red" role="status">
      <strong>Not found.</strong> LaunchDarkly has no flag with this key in {status.project}. It may have been removed
      since this step was written.
    </p>
  {:else if status.state === 'no-token' || status.state === 'disabled'}
    <p class="banner muted" role="status">{status.reason || 'Flag status is switched off.'}</p>
  {:else if !t}
    <p class="banner amber" role="status"><strong>Status unavailable.</strong> {status.reason}</p>
  {:else}
    {#if archived}
      <p class="banner amber" role="status"><strong>Archived.</strong> Nothing evaluates this flag any more.</p>
    {/if}
    <div class="panel" class:dim={archived}>
      <div class="line" role="status">
        <span class="env">{envName}</span>
        <span class="state" class:on><span class="dot"></span>{on ? 'On' : 'Off'}</span>
        <span class="gist">
          {#if !on}
            everyone gets <FlagServe serve={only(t.offVariation)} {variations} />
            {#if before}<span class="aside">({before} set up, not in effect)</span>{/if}
          {:else if before}
            {before}, then {#if split}a {split}-way rollout{:else}<FlagServe serve={t.fallthrough} {variations} />{/if}
          {:else if split}
            everyone is in a {split}-way rollout
          {:else}
            everyone gets <FlagServe serve={t.fallthrough} {variations} />
          {/if}
        </span>
      </div>

      {#if (before || split || variations.some((v) => pretty(v.value).block)) && !archived}
        <details class="decides" open={lines <= 3}>
          <summary>How {envName} decides</summary>
          <ol class="steps" class:idle={!on}>
            {#each t.prerequisites ?? [] as p (p.key)}
              <li>
                <span class="what"
                  ><span>Only if flag <code>{p.key}</code> serves its variation {p.variation + 1}</span></span
                >
              </li>
            {/each}
            {#each t.targets ?? [] as target, i (i)}
              <li>
                <span class="what"
                  >{target.count} {target.contextKind || 'user'}{target.count > 1 ? 's' : ''} named one by one</span
                >
                <span class="gets"
                  ><FlagServe serve={only(target.variation)} {variations} showValue unfolded={roomy} /></span
                >
              </li>
            {/each}
            {#each t.rules ?? [] as rule, i (i)}
              <li>
                <span class="what">
                  {#if rule.description}<span class="rule-name">{rule.description}</span>{/if}
                  {#each rule.clauses ?? [] as clause, j (j)}
                    {@const w = clauseWords(clause)}
                    <span class="clause"
                      >{j === 0 ? 'if' : 'and'}
                      {w.subject}
                      {w.verb}
                      {#each clause.values ?? [] as value, k (k)}{k > 0 ? ' or ' : ''}<code class:segment={w.segment}
                          >{value}</code
                        >{/each}</span
                    >
                    {#if w.segment}
                      {#each segmentsOf(clause.values) as segment (segment.key)}
                        <span class="segment-about">
                          <span class="segment-name">{segment.name || segment.key}</span>
                          {#if !segment.found}
                            <span class="red">couldn’t be read from LaunchDarkly</span>
                          {:else}
                            {segmentSize(segment)}
                            {#each segment.rules ?? [] as inner, r (r)}
                              <span class="segment-rule"
                                >{#each inner ?? [] as c, n (n)}{@const cw = clauseWords(c)}{n > 0
                                    ? ' and '
                                    : ''}{cw.subject}
                                  {cw.verb}
                                  {#each c.values ?? [] as value, k (k)}{k > 0 ? ' or ' : ''}<code>{value}</code
                                    >{/each}{/each}</span
                              >
                            {/each}
                          {/if}
                        </span>
                      {/each}
                    {/if}
                  {/each}
                </span>
                <span class="gets"><FlagServe serve={rule.serve} {variations} showValue unfolded={roomy} /></span>
              </li>
            {/each}
            <li>
              <span class="what">{before ? 'Everyone else' : 'Everyone'}</span>
              <span class="gets"><FlagServe serve={t.fallthrough} {variations} showValue unfolded={roomy} /></span>
            </li>
          </ol>
          {#if on && t.offVariation >= 0}
            <p class="off">If switched off: <FlagServe serve={only(t.offVariation)} {variations} /></p>
          {/if}
        </details>
      {/if}
    </div>

    {#if variations.some((v) => pretty(v.value).block)}
      <details class="all-variations">
        <summary>Variations ({variations.length})</summary>
        <ul class="variations">
          {#each variations as v (v.index)}
            {@const value = pretty(v.value)}
            <li>
              <span class="vname"
                ><span class="swatch {variationTone(variations, v.index)}"></span>{variationLabel(
                  variations,
                  v.index,
                )}</span
              >
              {#if value.block}<pre class="selectable">{value.text}</pre>{:else}<code>{value.text}</code>{/if}
            </li>
          {/each}
        </ul>
      </details>
    {/if}

    {#if pinned.length + rest.length > 0}
      <div class="others">
        <span class="also">Elsewhere</span>
        {#each allEnvs ? [...pinned, ...rest] : pinned as env (env.key)}
          <span class="other" class:on={env.on}
            ><span class="dot"></span>{env.name}<span class="sr">: {env.on ? 'on' : 'off'}</span></span
          >
        {/each}
        {#if rest.length > 0}
          <button class="more" onclick={() => (allEnvs = !allEnvs)}
            >{allEnvs ? 'fewer' : `+${rest.length} more (${rest.filter((e) => e.on).length} on)`}</button
          >
        {/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .flag-card {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px 13px;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--raised);
    font-family: var(--sans);
    font-size: 13px;
    line-height: 1.5;
    color: var(--text);
  }

  /* A colour per variation, shared with FlagServe's swatches and bars. */
  .flag-card :global(.v-true) {
    --tone: var(--green);
  }

  .flag-card :global(.v-false) {
    --tone: var(--muted);
  }

  .flag-card :global(.v0) {
    --tone: #6b7280;
  }

  .flag-card :global(.v1) {
    --tone: #4f7df5;
  }

  .flag-card :global(.v2) {
    --tone: #d9770b;
  }

  .flag-card :global(.v3) {
    --tone: #a855f7;
  }

  .flag-card :global(.v4) {
    --tone: #0d9488;
  }

  .head {
    display: flex;
    align-items: baseline;
    gap: 8px;
  }

  .title {
    font-weight: 600;
    font-size: 14px;
  }

  .type {
    font-size: 11px;
    padding: 0 6px;
    border-radius: 4px;
    background: var(--accent-soft);
    color: var(--accent);
    white-space: nowrap;
  }

  .out {
    margin-left: auto;
    border: 0;
    background: none;
    color: var(--muted);
    font-size: 11.5px;
    white-space: nowrap;
    padding: 0;
  }

  .out:hover:not(:disabled) {
    color: var(--accent);
  }

  .out:disabled {
    visibility: hidden;
  }

  .key {
    margin-top: -8px;
    font-family: var(--mono);
    font-size: 11.5px;
    color: var(--muted);
  }

  .about {
    margin: 0;
    color: var(--muted);
  }

  .panel {
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 8px 10px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .panel.dim {
    opacity: 0.55;
  }

  .panel.ghost {
    height: 40px;
    background: var(--border);
    animation: flag-pulse 1s ease-in-out infinite;
  }

  @keyframes flag-pulse {
    50% {
      opacity: 0.4;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .panel.ghost {
      animation: none;
    }
  }

  .line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 9px;
  }

  .env {
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: var(--accent);
  }

  .state {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-weight: 700;
    color: var(--muted);
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    box-shadow: inset 0 0 0 1.5px currentColor;
  }

  .state.on {
    color: var(--green);
  }

  .on > .dot {
    background: currentColor;
  }

  .gist {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 3px 6px;
    color: var(--muted);
    flex: 1 1 220px;
  }

  .aside {
    font-size: 11.5px;
  }

  details > summary {
    cursor: default;
    font-size: 11.5px;
    color: var(--muted);
    user-select: none;
  }

  details > summary:hover {
    color: var(--text);
  }

  .steps {
    margin: 6px 0 0;
    padding: 0;
    list-style: none;
    counter-reset: step;
  }

  .steps.idle {
    opacity: 0.55;
  }

  .steps li {
    counter-increment: step;
    display: grid;
    grid-template-columns: 18px 1fr;
    gap: 2px 6px;
    padding: 6px 0;
    border-top: 1px solid var(--border);
  }

  .steps li::before {
    content: counter(step);
    grid-row: 1 / span 2;
    font: 600 11px var(--mono);
    color: var(--muted);
    padding-top: 2px;
  }

  .what {
    display: flex;
    flex-direction: column;
  }

  .rule-name {
    font-weight: 600;
  }

  .clause {
    color: var(--muted);
  }

  .clause code,
  .what code {
    font-family: var(--mono);
    font-size: 11.5px;
    color: var(--text);
    padding: 0 4px;
    border-radius: 4px;
    background: var(--border);
  }

  .clause code.segment {
    background: var(--accent-soft);
    color: var(--accent);
  }

  .segment-about {
    display: flex;
    flex-direction: column;
    margin: 3px 0 3px 2px;
    padding-left: 8px;
    border-left: 2px solid var(--accent-soft);
    font-size: 11.5px;
    color: var(--muted);
  }

  .segment-name {
    font-weight: 600;
    color: var(--accent);
  }

  .segment-rule::before {
    content: '↳ ';
  }

  .segment-rule code {
    font-family: var(--mono);
    font-size: 11px;
    color: var(--text);
    padding: 0 3px;
    border-radius: 3px;
    background: var(--border);
  }

  .gets {
    min-width: 0;
    grid-column: 2;
    display: flex;
    align-items: flex-start;
    gap: 6px;
  }

  .gets::before {
    content: '→';
    color: var(--muted);
  }

  .off {
    margin: 4px 0 0;
    padding-top: 6px;
    border-top: 1px dashed var(--border);
    font-size: 11.5px;
    color: var(--muted);
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .variations {
    list-style: none;
    margin: 6px 0 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .vname {
    display: flex;
    align-items: center;
    gap: 6px;
    font-weight: 600;
  }

  .swatch {
    width: 8px;
    height: 8px;
    border-radius: 2px;
    background: var(--tone);
  }

  pre {
    margin: 3px 0 0;
    padding: 7px 9px;
    border-radius: 6px;
    background: var(--border);
    font: 11.5px/1.45 var(--mono);
    max-height: 132px;
    overflow: auto;
  }

  .others {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 10px;
    font-size: 11.5px;
    color: var(--muted);
  }

  .also {
    font-weight: 600;
  }

  .other {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .other .dot {
    width: 7px;
    height: 7px;
  }

  .other.on {
    color: var(--green);
  }

  .sr {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }

  .more {
    border: 0;
    background: none;
    padding: 0;
    font-size: 11.5px;
    color: var(--accent);
  }

  .banner {
    margin: 0;
    font-size: 12.5px;
  }

  .amber {
    color: var(--amber);
  }

  .red {
    color: var(--red);
  }

  .muted {
    color: var(--muted);
  }
</style>
