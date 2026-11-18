import { describe, expect, it } from 'vitest';
import { NavHistory } from './history';

describe('NavHistory', () => {
  it('walks back and forward through visited places', () => {
    const h = new NavHistory();
    h.visit({ path: '', index: 0 });
    h.visit({ path: 'a', index: 0 });
    h.visit({ path: 'a', index: 3 });

    expect(h.back()).toEqual({ path: 'a', index: 0 });
    expect(h.back()).toEqual({ path: '', index: 0 });
    expect(h.back()).toBeNull();
    expect(h.forward()).toEqual({ path: 'a', index: 0 });
    expect(h.forward()).toEqual({ path: 'a', index: 3 });
    expect(h.forward()).toBeNull();
  });

  it('drops what was ahead when visiting somewhere new', () => {
    const h = new NavHistory();
    h.visit({ path: 'a', index: 0 });
    h.visit({ path: 'a', index: 1 });
    h.back();
    h.visit({ path: 'b', index: 0 });

    expect(h.forward()).toBeNull();
    expect(h.back()).toEqual({ path: 'a', index: 0 });
  });

  it('ignores a repeat of the current place', () => {
    const h = new NavHistory();
    h.visit({ path: 'a', index: 0 });
    h.visit({ path: 'a', index: 0 });

    expect(h.back()).toBeNull();
  });

  it('moves the current entry while stepping, so back leaves the tour', () => {
    const h = new NavHistory();
    h.visit({ path: '', index: 0 });
    h.visit({ path: 'a', index: 4 });
    h.visit({ path: 'b', index: 0 });
    h.move({ path: 'b', index: 1 });
    h.move({ path: 'b', index: 2 });

    expect(h.back()).toEqual({ path: 'a', index: 4 });
    expect(h.forward()).toEqual({ path: 'b', index: 2 });
  });

  it('keeps a jump within a tour, and folds a step back onto it', () => {
    const h = new NavHistory();
    h.visit({ path: 'a', index: 0 });
    h.visit({ path: 'a', index: 5 });
    expect(h.canBack).toBe(true);

    h.move({ path: 'a', index: 6 });
    expect(h.back()).toEqual({ path: 'a', index: 0 });
    expect(h.forward()).toEqual({ path: 'a', index: 6 });

    h.move({ path: 'a', index: 0 });
    expect(h.canBack).toBe(false);
    expect(h.canForward).toBe(false);
  });

  it('starts an entry when there is nothing to move', () => {
    const h = new NavHistory();
    h.move({ path: 'a', index: 1 });
    h.visit({ path: 'b', index: 0 });

    expect(h.back()).toEqual({ path: 'a', index: 1 });
  });
});
