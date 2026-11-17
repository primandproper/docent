// Where the system map looks when it is zoomed in on a step.

export interface Box {
  x: number;
  y: number;
  width: number;
  height: number;
}

/** How much of its surroundings a focused part keeps around it, as a share of its longer side. */
const CONTEXT = 0.25;
/** And the least it keeps, in the diagram's own units, so a lone small node isn't blown up to fill the pane. */
const MARGIN = 24;

/** The smallest box around the centre of `box` that contains it and is `aspect` (width over height) in shape. */
export function fit(box: Box, aspect: number): Box {
  if (!(aspect > 0) || box.width <= 0 || box.height <= 0) {
    return box;
  }
  const width = Math.max(box.width, box.height * aspect);
  const height = width / aspect;
  return { x: box.x + (box.width - width) / 2, y: box.y + (box.height - height) / 2, width, height };
}

function union(a: Box, b: Box): Box {
  const x = Math.min(a.x, b.x);
  const y = Math.min(a.y, b.y);
  return {
    x,
    y,
    width: Math.max(a.x + a.width, b.x + b.width) - x,
    height: Math.max(a.y + a.height, b.y + b.height) - y,
  };
}

export function unionAll(boxes: Box[]): Box | null {
  return boxes.length === 0 ? null : boxes.reduce(union);
}

/** Slides a span of `size` starting at `start` so it stays inside [low, low + room], or centres it when it can't fit. */
function within(start: number, size: number, low: number, room: number): number {
  if (size >= room) {
    return low + (room - size) / 2;
  }
  return Math.max(low, Math.min(start, low + room - size));
}

/**
 * The part of a diagram to show when zoomed in on `focus`, for a viewport of
 * the given aspect: the focus with some of its surroundings, never more than
 * the whole diagram, and not hanging off its edge when it needn't.
 */
export function zoomTo(full: Box, focus: Box | null, aspect: number): Box {
  const whole = fit(full, aspect);
  if (!focus) {
    return whole;
  }

  const pad = Math.max(focus.width, focus.height) * CONTEXT + MARGIN;
  const box = fit(
    { x: focus.x - pad, y: focus.y - pad, width: focus.width + 2 * pad, height: focus.height + 2 * pad },
    aspect,
  );
  if (box.width >= whole.width) {
    return whole;
  }

  return {
    x: within(box.x, box.width, full.x, full.width),
    y: within(box.y, box.height, full.y, full.height),
    width: box.width,
    height: box.height,
  };
}

/** The box part of the way from one to another. */
export function between(from: Box, to: Box, t: number): Box {
  const mix = (a: number, b: number) => a + (b - a) * t;
  return {
    x: mix(from.x, to.x),
    y: mix(from.y, to.y),
    width: mix(from.width, to.width),
    height: mix(from.height, to.height),
  };
}

export function parseViewBox(value: string | null): Box | null {
  const parts = (value ?? '')
    .trim()
    .split(/[\s,]+/)
    .map(Number);
  if (parts.length !== 4 || parts.some((n) => !Number.isFinite(n))) {
    return null;
  }
  return { x: parts[0], y: parts[1], width: parts[2], height: parts[3] };
}

export function formatViewBox(box: Box): string {
  return [box.x, box.y, box.width, box.height].map((n) => Math.round(n * 100) / 100).join(' ');
}
