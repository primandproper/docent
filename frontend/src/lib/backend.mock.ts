// A recorded backend for browser tests: answers come from /fixture.json, which
// cmd/tools/fixture writes by running the real Go service over a workspace.
import { isExternalURL } from './api';
import type { Api, StepView, TourSummary, TourView, Workspace } from './api';

interface Fixture {
  workspace: Workspace;
  tours: TourSummary[];
  views: Record<string, TourView>;
  steps: Record<string, StepView[]>;
}

declare global {
  interface Window {
    /** URLs the app asked to open in the system browser, for tests to assert on. */
    openedURLs: string[];
  }
}

window.openedURLs = [];

let loaded: Promise<Fixture> | null = null;

function fixture(): Promise<Fixture> {
  loaded ??= fetch('/fixture.json').then((response) => response.json() as Promise<Fixture>);
  return loaded;
}

export const backend: Api = {
  workspace: async () => (await fixture()).workspace,
  listTours: async () => (await fixture()).tours,
  openTour: async (path) => {
    const view = (await fixture()).views[path];
    if (!view) {
      throw new Error(`${path}: not a tour in this workspace`);
    }
    return view;
  },
  step: async (path, index) => {
    const step = (await fixture()).steps[path]?.[index];
    if (!step) {
      throw new Error(`step ${index} of ${path}: no such step`);
    }
    return step;
  },
  openExternal: async (url) => {
    if (!isExternalURL(url)) {
      throw new Error(`refusing to open ${url}: only http and https links leave the app`);
    }
    window.openedURLs.push(url);
  },
};
