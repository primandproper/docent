import { api, errorMessage } from './api';
import type {
  FileView,
  MapMessage,
  MapState,
  ReviewState,
  StepView,
  TourSummary,
  TourView,
  VarView,
  Workspace,
} from './api';
import { NavHistory } from './history';
import { parseInternal } from './internal';
import type { Place } from './history';
import { parseStepNumber, positionAfterReload } from './steps';

/**
 * How long a step has to be on screen before it counts as read. Long enough
 * that holding `j` to the end of a tour reads nothing.
 */
export const READ_AFTER_MS = 1500;

/**
 * Everything the app is showing: the workspace, its tours, and the tour and
 * step being walked. Components read it and call its methods; nothing else
 * talks to Go.
 */
class Session {
  workspace = $state<Workspace | null>(null);
  tours = $state<TourSummary[]>([]);
  loading = $state(true);
  /** A failure worth interrupting the reader for. */
  error = $state('');
  /** Something that happened and is worth a passing mention. */
  notice = $state('');

  view = $state<TourView | null>(null);
  index = $state(0);
  step = $state<StepView | null>(null);
  vars = $state<VarView[]>([]);
  /** How far the reader is from being able to mark the open tour reviewed. */
  review = $state<ReviewState | null>(null);
  /** Whether the reader is being asked to confirm a review. */
  reviewAsking = $state(false);
  /** Whether a review is being saved. */
  reviewing = $state(false);

  /** Whether there is somewhere to go back or forward to. */
  canBack = $state(false);
  canForward = $state(false);

  private readTimer: ReturnType<typeof setTimeout> | undefined;
  private noticeTimer: ReturnType<typeof setTimeout> | undefined;
  private history = new NavHistory();
  private cache = new Map<number, StepView>();
  /** Counts navigations, so a slow answer for a step we already left is dropped. */
  private generation = 0;

  get path(): string {
    return this.view?.tour?.path ?? '';
  }

  get stepCount(): number {
    return this.view?.tour?.steps?.length ?? 0;
  }

  async init(): Promise<void> {
    try {
      this.workspace = await api.workspace();
      await this.refreshTours();
      this.record({ path: '', index: 0 });
      // Live reload: whoever is editing a tour sees it here as they save.
      api.onChanged(() => void this.reload());
      api.onMap((message) => this.heard(message));
      api.onMapWindowClosed(() => (this.mapPopped = false));
      // A docent:// link, now or later, opens its tour.
      api.onOpen(() => void this.launch());
      await this.launch();
    } catch (err) {
      this.error = errorMessage(err);
    } finally {
      this.loading = false;
    }
  }

  /** Opens whatever tour the app was asked for from outside, at the step asked for. */
  private async launch(): Promise<void> {
    const request = await api.launch();
    if (!request.tour) {
      return;
    }
    // A link may have moved the app to another folder.
    this.workspace = await api.workspace();
    await this.refreshTours();
    await this.open(request.tour);

    const steps = this.view?.tour?.steps ?? [];
    const byID = steps.findIndex((step) => step.id !== '' && step.id === request.step);
    const byNumber = parseStepNumber(request.step, steps.length);
    const index = byID >= 0 ? byID : (byNumber ?? 0);
    if (index > 0) {
      // Arriving at the step asked for is one move, not two.
      await this.go(index, 'move');
    }
  }

  /** Asks for a folder and opens it. */
  async chooseFolder(): Promise<void> {
    try {
      const workspace = await api.chooseFolder();
      if (workspace.root !== this.workspace?.root) {
        this.workspace = workspace;
        clearTimeout(this.readTimer);
        this.view = null;
        this.step = null;
        this.review = null;
        this.cache.clear();
      }
      await this.refreshTours();
    } catch (err) {
      this.error = errorMessage(err);
    }
  }

  async refreshTours(): Promise<void> {
    this.tours = await api.listTours();
  }

  /** Opens a tour at a step. */
  async open(path: string, index = 0, record: Recording = true): Promise<void> {
    try {
      const view = await api.openTour(path);
      this.cache.clear();
      this.view = view;
      this.vars = view.vars ?? [];
      this.review = view.review;
      this.error = '';
      await this.go(index, record);
    } catch (err) {
      this.error = errorMessage(err);
    }
  }

  /**
   * Moves to a step of the open tour. A jump is recorded in the history; `'move'` is for stepping, which takes the
   * current entry along instead.
   */
  async go(index: number, record: Recording = true): Promise<void> {
    const count = this.stepCount;
    clearTimeout(this.readTimer);
    if (!this.view || count === 0) {
      this.step = null;
      return;
    }

    const target = Math.max(0, Math.min(index, count - 1));
    const generation = ++this.generation;
    if (target !== this.index) {
      this.tab = -1;
    }
    this.browsing = null;
    this.editorMenu = false;
    this.reviewAsking = false;
    this.index = target;
    if (record) {
      this.record({ path: this.path, index: target }, record === 'move');
    }

    try {
      const step = await this.load(target);
      if (generation !== this.generation) {
        return;
      }
      this.step = step;
      this.error = '';
      this.countAsRead(target);
      // Warm the next step so `j` is instant.
      if (target + 1 < count) {
        void this.load(target + 1).catch(() => undefined);
      }
    } catch (err) {
      if (generation === this.generation) {
        this.error = errorMessage(err);
      }
    }
  }

  /**
   * Tells Go a step has been read, once it has stayed on screen for a moment.
   * Leaving the step first cancels it. Go keeps the count and is what refuses
   * a review of a tour that wasn't read through.
   */
  private countAsRead(index: number): void {
    clearTimeout(this.readTimer);
    if (!this.review?.available || this.review.read?.[index]) {
      return;
    }
    const path = this.path;
    this.readTimer = setTimeout(() => {
      if (path !== this.path || index !== this.index) {
        return;
      }
      // A step behind another window isn't being read: wait for it to come back.
      if (document.hidden) {
        this.countAsRead(index);
        return;
      }
      api
        .markRead(path, index)
        .then((review) => {
          if (path === this.path) {
            this.review = review;
          }
        })
        .catch((err: unknown) => (this.error = errorMessage(err)));
    }, READ_AFTER_MS);
  }

  /** Signs the open tour as reviewed. The caller has already asked the reader. */
  async markReviewed(name: string): Promise<boolean> {
    this.reviewing = true;
    try {
      await api.review(this.path, name);
      await this.reload();
      this.say(`Marked reviewed by ${name.trim()}.`);
      return true;
    } catch (err) {
      this.error = errorMessage(err);
      return false;
    } finally {
      this.reviewing = false;
    }
  }

  private async load(index: number): Promise<StepView> {
    const cached = this.cache.get(index);
    if (cached) {
      return cached;
    }
    const path = this.path;
    const step = await api.step(path, index);
    if (path === this.path) {
      this.cache.set(index, step);
    }
    return step;
  }

  next(): Promise<void> {
    return this.index + 1 < this.stepCount ? this.go(this.index + 1, 'move') : Promise.resolve();
  }

  previous(): Promise<void> {
    return this.index > 0 ? this.go(this.index - 1, 'move') : Promise.resolve();
  }

  /** Returns to the tour picker. */
  async close(record = true): Promise<void> {
    this.generation++;
    clearTimeout(this.readTimer);
    this.view = null;
    this.step = null;
    this.review = null;
    this.cache.clear();
    if (record) {
      this.record({ path: '', index: 0 });
    }
    try {
      await this.refreshTours();
    } catch (err) {
      this.error = errorMessage(err);
    }
  }

  /** Re-reads everything from disk, keeping the reader's place. */
  async reload(): Promise<void> {
    if (!this.view?.tour) {
      try {
        await this.refreshTours();
        this.error = '';
      } catch (err) {
        this.error = errorMessage(err);
      }
      return;
    }

    const previous = { id: this.view.tour.steps?.[this.index]?.id ?? '', index: this.index };
    try {
      const view = await api.openTour(this.path);
      this.cache.clear();
      this.view = view;
      this.vars = view.vars ?? [];
      this.review = view.review;
      this.error = '';
      await this.go(positionAfterReload(previous, view.tour?.steps ?? []), false);
    } catch (err) {
      this.error = errorMessage(err);
    }
  }

  /** Whether the system map is showing. The reader's choice, kept across tours. */
  mapShown = $state(true);
  /** Whether the map has the centre pane to itself, in place of the code. */
  mapLarge = $state(false);

  /** Gives the map the centre pane, or hands it back to the code. Showing it large also shows it. */
  toggleLargeMap(): void {
    this.mapLarge = !this.mapLarge;
    if (this.mapLarge) {
      this.mapShown = true;
    }
  }

  /** Whether the map is zoomed in on what the current step focuses. */
  mapZoom = $state(false);
  /** Whether the map is in a window of its own instead of in this one. */
  mapPopped = $state(false);

  /** Zooms the map in on the current step, or back out. Zooming a hidden map shows it. */
  toggleMapZoom(): void {
    this.mapZoom = !this.mapZoom;
    if (this.mapZoom) {
      this.mapShown = true;
    }
  }

  /** Pops the map out into its own window, or brings it back. */
  async toggleMapWindow(): Promise<void> {
    try {
      await api.toggleMapWindow();
    } catch (err) {
      this.error = errorMessage(err);
    }
  }

  /** What the map window should be drawing: null when the open tour has no map, or no tour is open. */
  get mapState(): MapState | null {
    const map = this.view?.map;
    if (!map) {
      return null;
    }
    return {
      title: this.view?.tour?.title ?? '',
      map,
      focus: (this.view?.tour?.steps ?? []).map((step) => step.mapFocus ?? []),
      index: this.index,
      zoomed: this.mapZoom,
    };
  }

  /** Tells the map window what to draw. */
  publishMap(): void {
    api.postMap({ kind: 'state', state: $state.snapshot(this.mapState) });
  }

  /** The map window asking for something. It holds no state of its own, so it can only ask. */
  private heard(message: MapMessage): void {
    if (message.kind === 'hello') {
      this.mapPopped = true;
      this.publishMap();
    } else if (!this.mapPopped || !this.view) {
      return;
    } else if (message.kind === 'go' && Number.isInteger(message.index)) {
      void this.go(message.index);
    } else if (message.kind === 'step') {
      void (message.by === 1 ? this.next() : this.previous());
    } else if (message.kind === 'zoom') {
      this.toggleMapZoom();
    }
  }

  /** Follows one of Docent's own links: to a step of this tour, or into another tour. */
  async follow(href: string): Promise<void> {
    const link = parseInternal(href);
    if (!link) {
      return;
    }

    if (link.kind === 'step') {
      const index = (this.view?.tour?.steps ?? []).findIndex((step) => step.id === link.id);
      if (index < 0) {
        this.error = `This tour has no step "${link.id}".`;
        return;
      }
      await this.go(index);
      return;
    }

    // A tour is named by the end of its path, so links survive wherever the tours folder lives.
    const target = this.tours.find((t) => t.path === link.path || t.path.endsWith(`/${link.path}`));
    if (!target || target.unreadable) {
      this.error = `There is no tour "${link.path}" in this workspace.`;
      return;
    }
    await this.open(target.path);
    if (link.id) {
      const index = (this.view?.tour?.steps ?? []).findIndex((step) => step.id === link.id);
      if (index > 0) {
        await this.go(index, 'move');
      }
    }
  }

  /** Picks a value for a link variable and re-resolves the links that use it. */
  async setVar(name: string, value: string): Promise<void> {
    try {
      this.vars = await api.setVar(this.path, name, value);
      this.cache.clear();
      await this.go(this.index, false);
    } catch (err) {
      this.error = errorMessage(err);
    }
  }

  /** Switches between reading code at each ref and reading it from disk. */
  async setWorkingTree(on: boolean): Promise<void> {
    try {
      await api.setWorkingTree(on);
      await this.reload();
    } catch (err) {
      this.error = errorMessage(err);
    }
  }

  /** What git is doing right now on the reader's say-so, as "fetch api", or ''. */
  gitBusy = $state('');

  /** Fetches or clones a repo of the open tour. The caller has already asked the reader. */
  async git(action: 'fetch' | 'clone', repo: string): Promise<void> {
    this.gitBusy = `${action} ${repo}`;
    try {
      await (action === 'fetch' ? api.fetch(this.path, repo) : api.clone(this.path, repo));
      await this.reload();
      this.say(action === 'fetch' ? `Fetched ${repo}.` : `Cloned ${repo}.`);
    } catch (err) {
      this.error = errorMessage(err);
    } finally {
      this.gitBusy = '';
    }
  }

  /** Which code the pane shows: -1 for the step's own, else the index of an extra anchor. */
  tab = $state(-1);
  /** Whether the file drawer is open. The reader's choice, kept across steps. */
  drawer = $state(false);
  /** A file opened from the drawer, shown in place of the step's code. */
  browsing = $state<FileView | null>(null);
  /** Whether the Open in… menu is showing. */
  editorMenu = $state(false);

  showTab(tab: number): void {
    this.tab = tab;
    this.browsing = null;
  }

  /** Shows a file from the drawer, or goes back to the step's code when given ''. */
  async browse(file: string): Promise<void> {
    if (!file) {
      this.browsing = null;
      return;
    }
    try {
      this.browsing = await api.browseFile(this.path, this.index, this.tab, file);
    } catch (err) {
      this.error = errorMessage(err);
    }
  }

  /** Opens the code the pane is showing in the reader's editor. */
  async openInEditor(editorID = ''): Promise<void> {
    if (this.tab < 0 && this.view?.steps?.[this.index]?.kind !== 'file') {
      return;
    }
    try {
      const result = await api.openInEditor(this.path, this.index, this.tab, editorID);
      const where =
        result.via === 'editor'
          ? `Opened in ${result.editor.name}`
          : result.via === 'default-app'
            ? 'Opened in the default app'
            : 'Revealed in Finder';
      this.say(result.note ? `${result.note}. ${where}.` : `${where}.`);
    } catch (err) {
      this.error = errorMessage(err);
    }
  }

  async copy(text: string, what: string): Promise<void> {
    try {
      await api.copy(text);
      this.say(`Copied ${what}.`);
    } catch (err) {
      this.error = errorMessage(err);
    }
  }

  say(message: string): void {
    this.notice = message;
    clearTimeout(this.noticeTimer);
    this.noticeTimer = setTimeout(() => (this.notice = ''), 3500);
  }

  back(): Promise<void> {
    return this.visit(this.history.back());
  }

  forward(): Promise<void> {
    return this.visit(this.history.forward());
  }

  private record(place: Place, moving = false): void {
    if (moving) {
      this.history.move(place);
    } else {
      this.history.visit(place);
    }
    this.canBack = this.history.canBack;
    this.canForward = this.history.canForward;
  }

  private visit(place: Place | null): Promise<void> {
    this.canBack = this.history.canBack;
    this.canForward = this.history.canForward;
    if (!place) {
      return Promise.resolve();
    }
    if (place.path === '') {
      return this.close(false);
    }
    return place.path === this.path ? this.go(place.index, false) : this.open(place.path, place.index, false);
  }
}

/** Whether a navigation is recorded in the history: as a new entry, by moving the current one, or not at all. */
type Recording = boolean | 'move';

export const session = new Session();
