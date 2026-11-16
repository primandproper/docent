// Mermaid, loaded on first use: it is the largest thing in the bundle, and a
// tour with no diagrams never pays for it.
import type { Mermaid } from 'mermaid';

let loading: Promise<Mermaid> | null = null;
let counter = 0;

function load(): Promise<Mermaid> {
  loading ??= import('mermaid').then((module) => module.default);
  return loading;
}

function token(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

/** A `click` directive whose target is one of Docent's own links. */
export interface DiagramClick {
  node: string;
  href: string;
}

const CLICK = /^\s*click\s+([\w.-]+)\s+(?:href\s+)?"([^"]*)".*$/;

/**
 * Takes the `click` directives out of a diagram. Mermaid's own click handling
 * is switched off (it can run script), so the ones that point at `docent:`
 * links are returned for the caller to wire up, and the rest are dropped.
 */
export function extractClicks(source: string): { source: string; clicks: DiagramClick[] } {
  const clicks: DiagramClick[] = [];
  const kept = source.split('\n').filter((line) => {
    if (!/^\s*click\s/.test(line)) {
      return true;
    }
    const match = CLICK.exec(line);
    if (match && match[2].startsWith('docent:')) {
      clicks.push({ node: match[1], href: match[2] });
    }
    return false;
  });
  return { source: kept.join('\n'), clicks };
}

export interface Rendered {
  /** The id Mermaid gave the SVG; element ids inside it are prefixed with it. */
  id: string;
  svg: string;
  clicks: DiagramClick[];
}

/**
 * Renders Mermaid source to SVG in the app's colors. Tours are untrusted, so
 * Mermaid runs at its strictest: no HTML in labels, no click handlers.
 */
export async function renderDiagram(source: string): Promise<Rendered> {
  const mermaid = await load();
  const extracted = extractClicks(source);

  mermaid.initialize({
    startOnLoad: false,
    securityLevel: 'strict',
    theme: 'base',
    fontFamily: token('--sans'),
    // Compact spacing: diagrams live in a side panel, and a tighter layout is
    // scaled down less, so its labels stay readable.
    flowchart: { nodeSpacing: 28, rankSpacing: 34, padding: 10, htmlLabels: false },
    themeVariables: {
      background: token('--bg'),
      primaryColor: token('--raised'),
      primaryTextColor: token('--text'),
      primaryBorderColor: token('--faint'),
      lineColor: token('--faint'),
      secondaryColor: token('--panel'),
      tertiaryColor: token('--panel'),
      clusterBkg: token('--panel'),
      clusterBorder: token('--border'),
      edgeLabelBackground: token('--bg'),
      fontSize: '13px',
    },
  });

  const id = `diagram-${++counter}`;
  const { svg } = await mermaid.render(id, extracted.source);
  return { id, svg, clicks: extracted.clicks };
}

/** The elements of a rendered flowchart, by the ids a tour's `mapFocus` uses. */
export function flowchartElements(root: Element, id: string): Map<string, Element[]> {
  const found = new Map<string, Element[]>();
  const add = (key: string, el: Element) => found.set(key, [...(found.get(key) ?? []), el]);

  // Nodes are `<id>-flowchart-<node>-<n>`, subgraphs `<id>-<subgraph>`, and
  // edges carry their id in data-id.
  root.querySelectorAll('g.node').forEach((el) => {
    const match = new RegExp(`^${id}-flowchart-(.+)-\\d+$`).exec(el.id);
    if (match) {
      add(match[1], el);
    }
  });
  root.querySelectorAll('g.cluster').forEach((el) => {
    if (el.id.startsWith(`${id}-`)) {
      add(el.id.slice(id.length + 1), el);
    }
  });
  root.querySelectorAll('path.flowchart-link[data-id]').forEach((el) => {
    add(el.getAttribute('data-id') ?? '', el);
  });
  return found;
}

/**
 * Calls back when the system switches between light and dark, so diagrams,
 * whose colors are baked in when they are drawn, can be drawn again.
 */
export function onSchemeChange(callback: () => void): () => void {
  const query = window.matchMedia('(prefers-color-scheme: dark)');
  query.addEventListener('change', callback);
  return () => query.removeEventListener('change', callback);
}
