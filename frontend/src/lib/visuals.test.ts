import { describe, expect, it } from 'vitest';
import { asVisual, flowFrames, nodes, stateFrames, timelineFrames } from './visuals';
import type { FlowVisual, StateVisual, TimelineVisual } from './visuals';

describe('nodes', () => {
  it('accepts bare ids and labelled objects', () => {
    expect(nodes(['api', { id: 'q', label: 'Queue' }, { id: 'w' }])).toEqual([
      { id: 'api', label: 'api' },
      { id: 'q', label: 'Queue' },
      { id: 'w', label: 'w' },
    ]);
  });
});

describe('asVisual', () => {
  it('passes a well-shaped visual through', () => {
    const v = { type: 'sequence', participants: ['a', 'b'], messages: [{ from: 'a', to: 'b', label: 'hi' }] };
    expect(asVisual(v)).toBe(v);
  });

  it('refuses anything it could not render', () => {
    expect(asVisual(null)).toBeNull();
    expect(asVisual('sequence')).toBeNull();
    expect(asVisual({ type: 'pie', slices: [1] })).toBeNull();
    expect(asVisual({ type: 'sequence', participants: ['a', 'b'] })).toBeNull();
    expect(asVisual({ type: 'flow', nodes: ['a'], edges: [], paths: [] })).toBeNull();
  });
});

describe('flowFrames', () => {
  it('is one hop per edge of each path, in order', () => {
    const v: FlowVisual = {
      type: 'flow',
      nodes: ['a', 'b', 'c'],
      edges: [
        { from: 'a', to: 'b' },
        { from: 'b', to: 'c' },
      ],
      paths: [
        { label: 'write', through: ['a', 'b', 'c'], step: 's1' },
        { label: 'read', through: ['a', 'b'] },
      ],
    };
    expect(flowFrames(v)).toEqual([
      { path: 0, label: 'write', from: 'a', to: 'b', step: 's1' },
      { path: 0, label: 'write', from: 'b', to: 'c', step: 's1' },
      { path: 1, label: 'read', from: 'a', to: 'b', step: undefined },
    ]);
  });
});

describe('stateFrames', () => {
  const v: StateVisual = {
    type: 'state',
    states: ['idle', 'busy', 'done'],
    transitions: [
      { from: 'idle', to: 'busy', on: 'start' },
      { from: 'busy', to: 'busy', on: 'retry' },
      { from: 'busy', to: 'done', on: 'finish' },
    ],
    run: ['start', 'retry', 'finish'],
  };

  it('follows the run from the first state', () => {
    expect(stateFrames(v)).toEqual([
      { event: 'start', from: 'idle', to: 'busy' },
      { event: 'retry', from: 'busy', to: 'busy' },
      { event: 'finish', from: 'busy', to: 'done' },
    ]);
  });

  it('stops at an event with nowhere to go', () => {
    expect(stateFrames({ ...v, run: ['start', 'start', 'finish'] })).toEqual([
      { event: 'start', from: 'idle', to: 'busy' },
      { event: 'start', from: 'busy', to: null },
    ]);
  });
});

describe('timelineFrames', () => {
  it('sorts events by time without touching the original', () => {
    const v: TimelineVisual = {
      type: 'timeline',
      lanes: ['w'],
      events: [
        { lane: 'w', at: 200, label: 'b' },
        { lane: 'w', at: 0, label: 'a' },
      ],
    };
    expect(timelineFrames(v).map((e) => e.label)).toEqual(['a', 'b']);
    expect(v.events[0].label).toBe('b');
  });
});
