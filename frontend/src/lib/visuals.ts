// The data behind the animated visuals: the shapes a tour declares, and the
// frames each one plays through. Everything here is plain data in and out.

export interface Node {
  id: string;
  label: string;
}

type RawNode = string | { id: string; label?: string };

export interface SequenceVisual {
  type: 'sequence';
  title?: string;
  participants: RawNode[];
  messages: { from: string; to: string; label: string; async?: boolean; step?: string }[];
}

export interface FlowVisual {
  type: 'flow';
  title?: string;
  nodes: RawNode[];
  edges: { from: string; to: string; label?: string }[];
  paths: { label: string; through: string[]; step?: string }[];
}

export interface StateVisual {
  type: 'state';
  title?: string;
  states: RawNode[];
  transitions: { from: string; to: string; on: string }[];
  run: string[];
}

export interface TimelineVisual {
  type: 'timeline';
  title?: string;
  unit?: string;
  lanes: RawNode[];
  events: { lane: string; at: number; label: string; step?: string }[];
}

export type Visual = SequenceVisual | FlowVisual | StateVisual | TimelineVisual;

export const VISUAL_TYPES = ['sequence', 'flow', 'state', 'timeline'] as const;

export function nodes(raw: RawNode[]): Node[] {
  return raw.map((n) => (typeof n === 'string' ? { id: n, label: n } : { id: n.id, label: n.label ?? n.id }));
}

/**
 * A light check that a decoded object is shaped like the visual its type
 * says, enough to render without throwing. `docent validate` does the real
 * checking; this only keeps a malformed visual from breaking the page.
 */
export function asVisual(value: unknown): Visual | null {
  if (typeof value !== 'object' || value === null) {
    return null;
  }
  const v = value as Record<string, unknown>;
  const lists: Record<string, string[]> = {
    sequence: ['participants', 'messages'],
    flow: ['nodes', 'edges', 'paths'],
    state: ['states', 'transitions', 'run'],
    timeline: ['lanes', 'events'],
  };
  const required = lists[String(v.type)];
  if (!required || !required.every((key) => Array.isArray(v[key]) && (v[key] as unknown[]).length > 0)) {
    return null;
  }
  return value as Visual;
}

/** One hop of a packet along a flow's path. */
export interface Hop {
  path: number;
  label: string;
  from: string;
  to: string;
  step?: string;
}

/** A flow plays each path in turn, one edge at a time. */
export function flowFrames(v: FlowVisual): Hop[] {
  return v.paths.flatMap((path, index) =>
    path.through
      .slice(1)
      .map((to, i) => ({ path: index, label: path.label, from: path.through[i], to, step: path.step })),
  );
}

/** One step of a state machine's run. `to` is where it lands; null means the event had nowhere to go. */
export interface Transition {
  event: string;
  from: string;
  to: string | null;
}

/** Plays a state machine's run from its first state. It stops at an event with no transition. */
export function stateFrames(v: StateVisual): Transition[] {
  const out: Transition[] = [];
  let current = nodes(v.states)[0]?.id ?? '';
  for (const event of v.run) {
    const taken = v.transitions.find((t) => t.from === current && t.on === event);
    out.push({ event, from: current, to: taken?.to ?? null });
    if (!taken) {
      break;
    }
    current = taken.to;
  }
  return out;
}

/** A timeline plays its events in time order, whatever order they were written in. */
export function timelineFrames(v: TimelineVisual): TimelineVisual['events'] {
  return [...v.events].sort((a, b) => a.at - b.at);
}
