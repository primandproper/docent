// A recorded backend for browser tests: answers come from /fixture.json, which
// cmd/tools/fixture writes by running the real Go service over a workspace.
import { isExternalURL } from './api';
import type { Api, Card, StepView, TourSummary, TourView, Workspace } from './api';

interface Fixture {
  workspace: Workspace;
  tours: TourSummary[];
  views: Record<string, TourView>;
  steps: Record<string, StepView[]>;
  /** Steps and views again for each choice of each variable, keyed "path|name=value". */
  picked: Record<string, { view: TourView; steps: StepView[] }>;
  /** What Go made of each docent-links fence, by tour path and then fence body. */
  linkFences: Record<string, Record<string, Card[]>>;
}

declare global {
  interface Window {
    /** What the app asked the outside world to do, for tests to assert on. */
    openedURLs: string[];
    openedInEditor: { path: string; index: number; editorID: string }[];
    copied: string[];
    gitActions: string[];
    /** The recorded answers. Tests edit this, then call docentChanged, to stand in for a file changing. */
    docentFixture?: Fixture;
    docentChanged: () => void;
  }
}

window.openedURLs = [];
window.openedInEditor = [];
window.copied = [];
window.gitActions = [];

const listeners = new Set<() => void>();
window.docentChanged = () => listeners.forEach((listener) => listener());

/** The variable choice in force per tour, as "name=value". */
const picks = new Map<string, string>();

async function fixture(): Promise<Fixture> {
  window.docentFixture ??= (await (await fetch('/fixture.json')).json()) as Fixture;
  return window.docentFixture;
}

async function current(path: string): Promise<{ view: TourView; steps: StepView[] } | undefined> {
  const f = await fixture();
  const pick = picks.get(path);
  if (pick && f.picked[`${path}|${pick}`]) {
    return f.picked[`${path}|${pick}`];
  }
  return f.views[path] ? { view: f.views[path], steps: f.steps[path] ?? [] } : undefined;
}

export const backend: Api = {
  workspace: async () => (await fixture()).workspace,
  listTours: async () => (await fixture()).tours,
  openTour: async (path) => {
    const found = await current(path);
    if (!found) {
      throw new Error(`${path}: not a tour in this workspace`);
    }
    return found.view;
  },
  step: async (path, index) => {
    const step = (await current(path))?.steps[index];
    if (!step) {
      throw new Error(`step ${index} of ${path}: no such step`);
    }
    return step;
  },
  setVar: async (path, name, value) => {
    if (value === '') {
      picks.delete(path);
    } else {
      picks.set(path, `${name}=${value}`);
    }
    return (await current(path))?.view.vars ?? [];
  },
  setWorkingTree: async (on) => {
    window.gitActions.push(`working-tree ${on}`);
    const f = await fixture();
    for (const view of Object.values(f.views)) {
      view.workingTree = on;
    }
  },
  fetch: async (_path, repo) => {
    window.gitActions.push(`fetch ${repo}`);
  },
  clone: async (_path, repo) => {
    window.gitActions.push(`clone ${repo}`);
  },
  expandLinks: async (path, body) => {
    const cards = (await fixture()).linkFences[path]?.[body];
    if (!cards) {
      throw new Error('this links fence was not recorded in the fixture');
    }
    return cards;
  },
  editors: async () => [{ id: 'zed', name: 'Zed' }],
  openInEditor: async (path, index, editorID) => {
    window.openedInEditor.push({ path, index, editorID });
    return { via: 'editor', editor: { id: 'zed', name: 'Zed' }, note: '' };
  },
  copy: async (text) => {
    window.copied.push(text);
  },
  onChanged: (callback) => {
    listeners.add(callback);
    return () => listeners.delete(callback);
  },
  openExternal: async (url) => {
    if (!isExternalURL(url)) {
      throw new Error(`refusing to open ${url}: only http and https links leave the app`);
    }
    window.openedURLs.push(url);
  },
};
