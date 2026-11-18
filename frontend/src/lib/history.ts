/** A place the reader has been: a step of a tour, or the tour picker (path ''). */
export interface Place {
  path: string;
  index: number;
}

function same(a: Place | undefined, b: Place): boolean {
  return a !== undefined && a.path === b.path && a.index === b.index;
}

/**
 * Browser-style navigation history: visiting somewhere new drops whatever was ahead.
 *
 * Stepping through a tour moves the current entry instead of adding one, the way scrolling a page does, so going
 * back from a tour reached by a link returns to the step the link was on, not through every step read since.
 */
export class NavHistory {
  private entries: Place[] = [];
  private cursor = -1;

  visit(place: Place): void {
    if (same(this.entries[this.cursor], place)) {
      return;
    }
    this.entries = this.entries.slice(0, this.cursor + 1);
    this.entries.push(place);
    this.cursor = this.entries.length - 1;
  }

  /** Stepping within a tour: the current entry follows the reader instead of a new one being added. */
  move(place: Place): void {
    if (this.cursor < 0 || this.entries[this.cursor].path !== place.path) {
      this.visit(place);
      return;
    }
    // Stepping onto the place just behind would leave a back that goes nowhere.
    if (same(this.entries[this.cursor - 1], place)) {
      this.entries.splice(this.cursor, 1);
      this.cursor -= 1;
      return;
    }
    this.entries[this.cursor] = place;
  }

  get canBack(): boolean {
    return this.cursor > 0;
  }

  get canForward(): boolean {
    return this.cursor < this.entries.length - 1;
  }

  back(): Place | null {
    if (this.cursor <= 0) {
      return null;
    }
    this.cursor -= 1;
    return this.entries[this.cursor];
  }

  forward(): Place | null {
    if (this.cursor >= this.entries.length - 1) {
      return null;
    }
    this.cursor += 1;
    return this.entries[this.cursor];
  }
}
