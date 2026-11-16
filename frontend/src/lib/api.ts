// The frontend's view of Go. Everything else imports from here; the calls
// themselves live in ./backend, which browser tests swap for a recorded one.
import type {
  StepView,
  TourSummary,
  TourView,
  Workspace,
} from '../../bindings/github.com/primandproper/docent/internal/app';
import { backend } from './backend';

export type {
  StepSummary,
  StepView,
  TourSummary,
  TourView,
  Workspace,
} from '../../bindings/github.com/primandproper/docent/internal/app';
export type { Location, Range } from '../../bindings/github.com/primandproper/docent/internal/anchor';
export type { Diagnostic } from '../../bindings/github.com/primandproper/docent/internal/diag';
export type { Entry } from '../../bindings/github.com/primandproper/docent/internal/gitsrc';
export type { Step, Tour } from '../../bindings/github.com/primandproper/docent/internal/tour';

export interface Api {
  workspace(): Promise<Workspace>;
  listTours(): Promise<TourSummary[]>;
  openTour(path: string): Promise<TourView>;
  step(path: string, index: number): Promise<StepView>;
  /** Opens an http(s) URL in the system browser. Anything else is refused. */
  openExternal(url: string): Promise<void>;
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
