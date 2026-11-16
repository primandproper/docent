import { api, errorMessage } from './api';
import type { StepView, TourSummary, TourView, Workspace } from './api';
import { NavHistory } from './history';
import type { Place } from './history';
import { positionAfterReload } from './steps';

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

  view = $state<TourView | null>(null);
  index = $state(0);
  step = $state<StepView | null>(null);

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
      this.history.visit({ path: '', index: 0 });
    } catch (err) {
      this.error = errorMessage(err);
    } finally {
      this.loading = false;
    }
  }

  async refreshTours(): Promise<void> {
    this.tours = await api.listTours();
  }

  /** Opens a tour at a step. */
  async open(path: string, index = 0, record = true): Promise<void> {
    try {
      const view = await api.openTour(path);
      this.cache.clear();
      this.view = view;
      this.error = '';
      await this.go(index, record);
    } catch (err) {
      this.error = errorMessage(err);
    }
  }

  /** Moves to a step of the open tour. */
  async go(index: number, record = true): Promise<void> {
    const count = this.stepCount;
    if (!this.view || count === 0) {
      this.step = null;
      return;
    }

    const target = Math.max(0, Math.min(index, count - 1));
    const generation = ++this.generation;
    this.index = target;
    if (record) {
      this.history.visit({ path: this.path, index: target });
    }

    try {
      const step = await this.load(target);
      if (generation !== this.generation) {
        return;
      }
      this.step = step;
      this.error = '';
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
    return this.index + 1 < this.stepCount ? this.go(this.index + 1) : Promise.resolve();
  }

  previous(): Promise<void> {
    return this.index > 0 ? this.go(this.index - 1) : Promise.resolve();
  }

  /** Returns to the tour picker. */
  async close(record = true): Promise<void> {
    this.generation++;
    this.view = null;
    this.step = null;
    this.cache.clear();
    if (record) {
      this.history.visit({ path: '', index: 0 });
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
      this.error = '';
      await this.go(positionAfterReload(previous, view.tour?.steps ?? []), false);
    } catch (err) {
      this.error = errorMessage(err);
    }
  }

  back(): Promise<void> {
    return this.visit(this.history.back());
  }

  forward(): Promise<void> {
    return this.visit(this.history.forward());
  }

  private visit(place: Place | null): Promise<void> {
    if (!place) {
      return Promise.resolve();
    }
    if (place.path === '') {
      return this.close(false);
    }
    return place.path === this.path ? this.go(place.index, false) : this.open(place.path, place.index, false);
  }
}

export const session = new Session();
