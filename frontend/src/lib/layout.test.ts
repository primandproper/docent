import { describe, expect, it } from 'vitest';
import { layoutGraph, pointAt, toPath } from './layout';

describe('pointAt', () => {
  const corner = [
    { x: 0, y: 0 },
    { x: 10, y: 0 },
    { x: 10, y: 30 },
  ];

  it('measures along the whole polyline', () => {
    expect(pointAt(corner, 0)).toEqual({ x: 0, y: 0 });
    expect(pointAt(corner, 0.25)).toEqual({ x: 10, y: 0 });
    expect(pointAt(corner, 0.5)).toEqual({ x: 10, y: 10 });
    expect(pointAt(corner, 1)).toEqual({ x: 10, y: 30 });
  });

  it('clamps, and survives degenerate input', () => {
    expect(pointAt(corner, 2)).toEqual({ x: 10, y: 30 });
    expect(pointAt(corner, -1)).toEqual({ x: 0, y: 0 });
    expect(pointAt([], 0.5)).toEqual({ x: 0, y: 0 });
    expect(pointAt([{ x: 4, y: 4 }], 0.5)).toEqual({ x: 4, y: 4 });
  });
});

describe('toPath', () => {
  it('writes an SVG path', () => {
    expect(
      toPath([
        { x: 1, y: 2 },
        { x: 3, y: 4 },
      ]),
    ).toBe('M1,2 L3,4');
  });
});

describe('layoutGraph', () => {
  it('places every node and routes every edge, left to right', async () => {
    const layout = await layoutGraph(
      [
        { id: 'a', label: 'A' },
        { id: 'b', label: 'A longer label' },
        { id: 'c', label: 'C' },
      ],
      [
        { from: 'a', to: 'b', label: 'go' },
        { from: 'b', to: 'c' },
      ],
    );

    expect(layout.nodes.map((n) => n.id)).toEqual(['a', 'b', 'c']);
    const [a, b, c] = layout.nodes;
    expect(a.x).toBeLessThan(b.x);
    expect(b.x).toBeLessThan(c.x);
    expect(b.width).toBeGreaterThan(a.width);
    expect(layout.edges).toHaveLength(2);
    expect(layout.edges[0].points.length).toBeGreaterThanOrEqual(2);
    expect(layout.edges[0].label).toBe('go');
    expect(layout.width).toBeGreaterThan(0);
  });
});
