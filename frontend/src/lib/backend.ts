// The real backend: Wails bindings into the Go service. Building with
// `--mode mock` swaps this module for ./backend.mock.
import { Browser, Clipboard, Events } from '@wailsio/runtime';
import { Service } from '../../bindings/github.com/primandproper/docent/internal/app';
import { isExternalURL } from './api';
import type { Api, MapMessage } from './api';

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
  setWorkingTree: (on) => Service.SetWorkingTree(on),
  fetch: (path, repo) => Service.Fetch(path, repo),
  clone: (path, repo) => Service.Clone(path, repo),
  markRead: async (path, index) => required(await Service.MarkRead(path, index), 'review state'),
  review: (path, name) => Service.Review(path, name),
  expandLinks: async (path, body) => (await Service.ExpandLinks(path, body)) ?? [],
  editors: async () => (await Service.Editors()) ?? [],
  openInEditor: async (path, index, extra, editorID) =>
    required(await Service.OpenInEditor(path, index, extra, editorID), 'result'),
  browse: async (path, index, extra, dir) => (await Service.Browse(path, index, extra, dir)) ?? [],
  browseFile: async (path, index, extra, file) => required(await Service.BrowseFile(path, index, extra, file), 'file'),
  copy: (text) => Clipboard.SetText(text),
  launch: () => Service.Launch(),
  chooseFolder: () => Service.ChooseFolder(),
  // Must match app.OpenEvent in Go.
  onOpen: (callback) => Events.On('docent:open' as never, callback),
  // Must match app.ChangedEvent in Go.
  onChanged: (callback) => Events.On('docent:changed' as never, callback),
  toggleMapWindow: () => Service.ToggleMapWindow(),
  // Must match app.MapClosedEvent in Go.
  onMapWindowClosed: (callback) => Events.On('docent:map-closed' as never, callback),
  // Must match app.MapEvent in Go. Wails hands an event from one window to all of them.
  postMap: (message) => void Events.Emit('docent:map', message),
  onMap: (callback) => Events.On('docent:map', (event) => callback(event.data as MapMessage)),
  openExternal: async (url) => {
    if (!isExternalURL(url)) {
      throw new Error(`refusing to open ${url}: only http and https links leave the app`);
    }
    await Browser.OpenURL(url);
  },
};
