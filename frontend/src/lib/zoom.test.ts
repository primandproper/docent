import { describe, expect, it } from 'vitest';
import { between, fit, formatViewBox, parseViewBox, unionAll, zoomTo } from './zoom';

const full = { x: 0, y: 0, width: 1000, height: 400 };

describe('fit', () => {
  it('grows a box about its centre to the shape asked for', () => {
    expect(fit({ x: 0, y: 0, width: 100, height: 100 }, 2)).toEqual({ x: -50, y: 0, width: 200, height: 100 });
    expect(fit({ x: 0, y: 0, width: 100, height: 100 }, 0.5)).toEqual({ x: 0, y: -50, width: 100, height: 200 });
  });
});

describe('zoomTo', () => {
  it('shows the whole diagram when the step focuses nothing', () => {
    expect(zoomTo(full, null, 2.5)).toEqual(full);
  });

  it('closes in on the focus, keeping the viewport shape and some surroundings', () => {
    const box = zoomTo(full, { x: 450, y: 180, width: 100, height: 40 }, 2);

    expect(box.width / box.height).toBeCloseTo(2);
    expect(box.width).toBeLessThan(full.width / 2);
    expect(box.x).toBeLessThan(450);
    expect(box.x + box.width).toBeGreaterThan(550);
    expect(box.y).toBeLessThan(180);
    expect(box.y + box.height).toBeGreaterThan(220);
  });

  it('stays inside the diagram for a focus at its edge', () => {
    const box = zoomTo(full, { x: 0, y: 0, width: 80, height: 40 }, 2);

    expect(box.x).toBe(0);
    expect(box.y).toBe(0);
  });

  it('never shows more than the whole diagram', () => {
    expect(zoomTo(full, { x: 10, y: 10, width: 980, height: 380 }, 2.5)).toEqual(full);
  });
});

describe('helpers', () => {
  it('joins boxes', () => {
    expect(unionAll([])).toBeNull();
    expect(
      unionAll([
        { x: 0, y: 0, width: 10, height: 10 },
        { x: 20, y: 5, width: 10, height: 10 },
      ]),
    ).toEqual({ x: 0, y: 0, width: 30, height: 15 });
  });

  it('moves between boxes', () => {
    expect(between(full, { x: 100, y: 100, width: 500, height: 200 }, 0.5)).toEqual({
      x: 50,
      y: 50,
      width: 750,
      height: 300,
    });
  });

  it('reads and writes a viewBox', () => {
    expect(parseViewBox('0 0 1000.5 400')).toEqual({ x: 0, y: 0, width: 1000.5, height: 400 });
    expect(parseViewBox('')).toBeNull();
    expect(formatViewBox({ x: 1.234, y: 0, width: 10, height: 5 })).toBe('1.23 0 10 5');
  });
});
