// The real backend: Wails bindings into the Go service. Building with
// `--mode mock` swaps this module for ./backend.mock.
import { Browser, Clipboard, Events } from '@wailsio/runtime';
import { Service } from '../../bindings/github.com/primandproper/docent/internal/app';
import { isExternalURL } from './api';
import type { Api } from './api';

function required<T>(value: T | null, what: string): T {
  if (value === null) {
    throw new Error(`Docent returned no ${what}`);
  }
  return value;
}

export const backend: Api = {
  workspace: () => Service.Workspace(),
  listTours: async () => (await Service.ListTours()) ?? [],
  openTour: async (path) => required(await Service.OpenTour(path), 'tour'),
  step: async (path, index) => required(await Service.Step(path, index), 'step'),
  setVar: async (path, name, value) => (await Service.SetVar(path, name, value)) ?? [],
  editors: async () => (await Service.Editors()) ?? [],
  openInEditor: async (path, index, editorID) => required(await Service.OpenInEditor(path, index, editorID), 'result'),
  copy: (text) => Clipboard.SetText(text),
  // Must match app.ChangedEvent in Go.
  onChanged: (callback) => Events.On('docent:changed' as never, callback),
  openExternal: async (url) => {
    if (!isExternalURL(url)) {
      throw new Error(`refusing to open ${url}: only http and https links leave the app`);
    }
    await Browser.OpenURL(url);
  },
};
