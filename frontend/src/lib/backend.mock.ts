// A recorded backend for browser tests: answers come from /fixture.json, which
// cmd/tools/fixture writes by running the real Go service over a workspace.
import { isExternalURL } from './api';
import type { Status as FlagStatus } from './flags';
import type {
  Api,
  Card,
  Entry,
  MapMessage,
  FileView,
  Folder,
  ReviewState,
  StepView,
  TourSummary,
  TourView,
} from './api';

interface Fixture {
  /** Every recorded folder. Only the first is open to begin with. */
  folders: Folder[];
  tours: TourSummary[];
  views: Record<string, TourView>;
  steps: Record<string, StepView[]>;
  /** Steps and views again for each choice of each variable, keyed "path|name=value". */
  picked: Record<string, { view: TourView; steps: StepView[] }>;
  /** A few recorded directory listings and files of each tour's first repo, for the file drawer. */
  listings: Record<string, Record<string, Entry[]>>;
  files: Record<string, Record<string, FileView>>;
  /** What Go made of each docent-links fence, by tour path and then fence body. */
  linkFences: Record<string, Record<string, Card[]>>;
  /** What, other than unread steps, stops each tour being reviewed, by tour path. '' when nothing does. */
  reviewProblems: Record<string, string>;
  /** Each flag card's status, by tour path (or "path|name=value" for a picked variable) and then "flag|project". */
  flags: Record<string, Record<string, FlagStatus>>;
}

declare global {
  interface Window {
    /** What the app asked the outside world to do, for tests to assert on. */
    openedURLs: string[];
    openedInEditor: { path: string; index: number; extra: number; editorID: string }[];
    copied: string[];
    gitActions: string[];
    /** The flags whose status was asked for, in order. */
    flagsAsked: string[];
    /** The recorded answers. Tests edit this, then call docentChanged, to stand in for a file changing. */
    docentFixture?: Fixture;
    docentChanged: () => void;
    docentLaunch?: { tour: string; step: string };
    /** The roots of the folders to start with, in place of the first recorded one. */
    docentFolders?: string[];
    /** The map window, once it has been popped out. */
    docentMapWindow?: Window | null;
    /** Reviews the app signed, as "name: path". */
    reviewed: string[];
  }
}

window.openedURLs = [];
window.openedInEditor = [];
window.copied = [];
window.gitActions = [];
window.flagsAsked = [];
window.reviewed = [];

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

/** The roots of the folders that are open. It starts as `docent <folder>` would: one folder, not saved. */
let openRoots: string[] | undefined;
/** The roots of the folders the reader added or kept. */
const saved = new Set<string>();

async function roots(): Promise<string[]> {
  openRoots ??= window.docentFolders ?? (await fixture()).folders.slice(0, 1).map((folder) => folder.root);
  return openRoots;
}

async function openFolders(): Promise<Folder[]> {
  const open = await roots();
  const recorded = (await fixture()).folders;
  return open.flatMap((root) => {
    const folder = recorded.find((f) => f.root === root);
    return folder ? [{ ...folder, saved: saved.has(root) }] : [];
  });
}

/** Which steps of each tour have been read, as Go would keep it. */
const read = new Map<string, boolean[]>();

/** The reviewer's name Go would have found in git's config. */
const READER = 'Ada Reader';

async function reviewState(path: string, view: TourView): Promise<ReviewState> {
  const steps = view.tour?.steps?.length ?? 0;
  let seen = read.get(path);
  if (!seen || seen.length !== steps) {
    seen = new Array<boolean>(steps).fill(false);
    read.set(path, seen);
  }
  const available = view.review.available;
  const problem = (await fixture()).reviewProblems[path] ?? '';
  const blocker = problem || (seen.every(Boolean) ? '' : 'Read every step first.');
  return { available, name: available ? READER : '', read: [...seen], blocker, ready: available && blocker === '' };
}

// The map window is a second page here, and the two talk over
// BroadcastChannels, as they do over Wails events in the app.
const IS_MAP_WINDOW = location.hash === '#map';
const mapChannel = new BroadcastChannel('docent:map');
const mapClosedChannel = new BroadcastChannel('docent:map-closed');

if (IS_MAP_WINDOW) {
  window.addEventListener('pagehide', () => mapClosedChannel.postMessage(null));
}

export const backend: Api = {
  toggleMapWindow: async () => {
    if (IS_MAP_WINDOW) {
      window.close();
    } else if (!window.docentMapWindow || window.docentMapWindow.closed) {
      window.docentMapWindow = window.open('/#map', 'docent-map', 'popup,width=560,height=420');
    } else {
      window.docentMapWindow.close();
    }
  },
  onMapWindowClosed: (callback) => {
    const listener = () => callback();
    mapClosedChannel.addEventListener('message', listener);
    return () => mapClosedChannel.removeEventListener('message', listener);
  },
  postMap: (message) => mapChannel.postMessage(message),
  onMap: (callback) => {
    const listener = (event: MessageEvent) => callback(event.data as MapMessage);
    mapChannel.addEventListener('message', listener);
    return () => mapChannel.removeEventListener('message', listener);
  },
  folders: openFolders,
  listTours: async () => {
    const open = await roots();
    const tours = (await fixture()).tours;
    return open.flatMap((root) => tours.filter((tour) => tour.folder === root));
  },
  openTour: async (path) => {
    const found = await current(path);
    const summary = (await fixture()).tours.find((tour) => tour.path === path);
    if (!found || !summary || !(await roots()).includes(summary.folder)) {
      throw new Error(`${path}: not a tour in a folder that is open`);
    }
    return { ...found.view, review: await reviewState(path, found.view) };
  },
  markRead: async (path, index) => {
    const found = await current(path);
    if (!found) {
      throw new Error(`${path} is not open`);
    }
    await reviewState(path, found.view);
    read.get(path)![index] = true;
    return reviewState(path, found.view);
  },
  review: async (path, name) => {
    const found = await current(path);
    if (!found?.view.tour || !(await reviewState(path, found.view)).ready) {
      throw new Error("this tour hasn't been read through yet");
    }
    window.reviewed.push(`${name}: ${path}`);
    // Stand in for what Go writes: the review, and a read that starts over.
    const f = await fixture();
    const reviews = [...(found.view.tour.reviewedBy ?? []), { name, at: '2026-11-16T12:00:00Z', ref: '' }];
    for (const view of [
      f.views[path],
      ...Object.entries(f.picked).map(([key, value]) => (key.startsWith(`${path}|`) ? value.view : undefined)),
    ]) {
      if (view?.tour) {
        view.tour.reviewedBy = reviews;
      }
    }
    read.delete(path);
    window.docentChanged();
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
  flagStatus: async (path, flag, project, fresh) => {
    window.flagsAsked.push(`${flag}${fresh ? ' (fresh)' : ''}`);
    const pick = picks.get(path);
    const status = (await fixture()).flags[pick ? `${path}|${pick}` : path]?.[`${flag}|${project}`];
    if (!status) {
      throw new Error(`flag ${flag} was not recorded in the fixture`);
    }
    return status;
  },
  expandLinks: async (path, body) => {
    const cards = (await fixture()).linkFences[path]?.[body];
    if (!cards) {
      throw new Error('this links fence was not recorded in the fixture');
    }
    return cards;
  },
  editors: async () => [
    { id: 'zed', name: 'Zed' },
    { id: 'code', name: 'VS Code' },
  ],
  browse: async (path, _index, _extra, dir) => {
    const entries = (await fixture()).listings[path]?.[dir];
    if (!entries) {
      throw new Error(`${dir || 'the root'} was not recorded in the fixture`);
    }
    return entries;
  },
  browseFile: async (path, _index, _extra, file) => {
    const found = (await fixture()).files[path]?.[file];
    if (!found) {
      throw new Error(`${file} was not recorded in the fixture`);
    }
    return found;
  },
  openInEditor: async (path, index, extra, editorID) => {
    window.openedInEditor.push({ path, index, extra, editorID });
    return {
      via: 'editor',
      editor: editorID === 'code' ? { id: 'code', name: 'VS Code' } : { id: 'zed', name: 'Zed' },
      note: '',
    };
  },
  copy: async (text) => {
    window.copied.push(text);
  },
  // Tests set window.docentLaunch before the app starts to stand in for `docent open`.
  launch: async () => window.docentLaunch ?? { tour: '', step: '' },
  // The folder "picked in the dialog" is the next recorded one that isn't open.
  addFolder: async () => {
    window.gitActions.push('add-folder');
    const open = await roots();
    const next = (await fixture()).folders.find((folder) => !open.includes(folder.root));
    if (next) {
      open.push(next.root);
      saved.add(next.root);
    }
    return openFolders();
  },
  keepFolder: async (root) => {
    if (!(await roots()).includes(root)) {
      throw new Error(`${root}: not a folder that is open`);
    }
    saved.add(root);
    return openFolders();
  },
  removeFolder: async (root) => {
    const open = await roots();
    if (!open.includes(root)) {
      throw new Error(`${root}: not a folder that is open`);
    }
    open.splice(open.indexOf(root), 1);
    saved.delete(root);
    return openFolders();
  },
  onOpen: () => () => undefined,
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
