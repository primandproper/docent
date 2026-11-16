/** A place the reader has been: a step of a tour, or the tour picker (path ''). */
export interface Place {
  path: string;
  index: number;
}

function same(a: Place | undefined, b: Place): boolean {
  return a !== undefined && a.path === b.path && a.index === b.index;
}

/** Browser-style navigation history: visiting somewhere new drops whatever was ahead. */
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
