// The frontend's view of Go. Everything else imports from here; the calls
// themselves live in ./backend, which browser tests swap for a recorded one.
import type {
  FileView,
  ReviewState,
  StepView,
  TourSummary,
  TourView,
  VarView,
  Workspace,
} from '../../bindings/github.com/primandproper/docent/internal/app';
import type { Editor, Result as EditorResult } from '../../bindings/github.com/primandproper/docent/internal/editor';
import type { Entry } from '../../bindings/github.com/primandproper/docent/internal/gitsrc';
import type { Card } from '../../bindings/github.com/primandproper/docent/internal/links';
import { backend } from './backend';

export type {
  AnchorView,
  FileView,
  ReviewState,
  StepSummary,
  StepView,
  TourSummary,
  TourView,
  VarView,
  Workspace,
} from '../../bindings/github.com/primandproper/docent/internal/app';
export type { Editor, Result as EditorResult } from '../../bindings/github.com/primandproper/docent/internal/editor';
export type { RepoState, Ref } from '../../bindings/github.com/primandproper/docent/internal/walk';
export type { Card } from '../../bindings/github.com/primandproper/docent/internal/links';
export type { Location, Range } from '../../bindings/github.com/primandproper/docent/internal/anchor';
export type { Diagnostic } from '../../bindings/github.com/primandproper/docent/internal/diag';
export type { Entry } from '../../bindings/github.com/primandproper/docent/internal/gitsrc';
export type { Step, Tour } from '../../bindings/github.com/primandproper/docent/internal/tour';

/** What the map window needs to draw the map and follow the tour. */
export interface MapState {
  title: string;
  /** The map's Mermaid source. */
  map: string;
  /** Each step's `mapFocus`, in step order. */
  focus: string[][];
  index: number;
  zoomed: boolean;
}

/**
 * What the main window and the map window say to each other. The main window
 * holds all the state: the map window draws what it is told and asks for the
 * rest.
 */
export type MapMessage =
  /** Map window: I am open, tell me what to draw. */
  | { kind: 'hello' }
  /** Main window: here is the map, or null when there is none to show. */
  | { kind: 'state'; state: MapState | null }
  /** Map window: the reader asked for a step, the one after or before, or to zoom. */
  | { kind: 'go'; index: number }
  | { kind: 'step'; by: 1 | -1 }
  | { kind: 'zoom' };

export interface Api {
  workspace(): Promise<Workspace>;
  listTours(): Promise<TourSummary[]>;
  openTour(path: string): Promise<TourView>;
  step(path: string, index: number): Promise<StepView>;
  /** Records a link variable picked in the app; returns the tour's variables as they now stand. */
  setVar(path: string, name: string, value: string): Promise<VarView[]>;
  /** Reads code as it is on disk (true) or at each repo's ref (false). */
  setWorkingTree(on: boolean): Promise<void>;
  /** Runs git fetch in one of the open tour's repos. Call only once the reader has agreed. */
  fetch(path: string, repo: string): Promise<void>;
  /** Clones one of the open tour's missing repos. Call only once the reader has agreed. */
  clone(path: string, repo: string): Promise<void>;
  /** Records that a step has been in front of the reader; returns what a review still waits on. */
  markRead(path: string, index: number): Promise<ReviewState>;
  /**
   * Signs the open tour as reviewed, which edits the tour file. Call only once the reader has
   * agreed. Go refuses unless every step was read first.
   */
  review(path: string, name: string): Promise<void>;
  /** Turns the body of a docent-links fence into cards, with variables filled in. */
  expandLinks(path: string, body: string): Promise<Card[]>;
  editors(): Promise<Editor[]>;
  /**
   * Opens a file step in an editor: the one with this id, or the reader's
   * choice for the file when empty. `extra` is an extra anchor's index, or -1.
   */
  openInEditor(path: string, index: number, extra: number, editorID: string): Promise<EditorResult>;
  /** Lists a directory of the repo a step's code is in. `extra` is an extra anchor's index, or -1. */
  browse(path: string, index: number, extra: number, dir: string): Promise<Entry[]>;
  browseFile(path: string, index: number, extra: number, file: string): Promise<FileView>;
  /** Opens an http(s) URL in the system browser. Anything else is refused. */
  openExternal(url: string): Promise<void>;
  copy(text: string): Promise<void>;
  /** The tour the app was asked to open from outside (`docent open`, a docent:// link), once. */
  launch(): Promise<{ tour: string; step: string }>;
  /** Asks the reader for a folder and opens it. Resolves to the workspace as it now is. */
  chooseFolder(): Promise<Workspace>;
  /** Gives the system map a window of its own, floating above this one, or closes that window if it is open. */
  toggleMapWindow(): Promise<void>;
  /** Calls back when the map window has closed, whoever closed it. */
  onMapWindowClosed(callback: () => void): () => void;
  /** Says something to the other window. */
  postMap(message: MapMessage): void;
  /** Calls back with what the other window says. A window may hear itself. */
  onMap(callback: (message: MapMessage) => void): () => void;
  /** Calls back when a docent:// link arrives while the app is running. */
  onOpen(callback: () => void): () => void;
  /** Calls back whenever a tour changes on disk. Returns a function that stops it. */
  onChanged(callback: () => void): () => void;
}

export function isExternalURL(url: string): boolean {
  try {
    const { protocol } = new URL(url);
    return protocol === 'http:' || protocol === 'https:';
  } catch {
    return false;
  }
}

export const api: Api = backend;

/** Turns whatever a rejected binding call threw into a sentence. */
export function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return typeof err === 'string' ? err : JSON.stringify(err);
}
