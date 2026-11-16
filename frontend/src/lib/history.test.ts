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
});
