// Graph layout for the flow and state visuals, by ELK. Loaded on first use.
import type { ELK, ElkNode } from 'elkjs/lib/elk-api';
import type { Node } from './visuals';

export interface Point {
  x: number;
  y: number;
}

export interface PlacedNode extends Node {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface PlacedEdge {
  from: string;
  to: string;
  label: string;
  points: Point[];
  /** Where the label sits. */
  at: Point;
}

export interface Layout {
  width: number;
  height: number;
  nodes: PlacedNode[];
  edges: PlacedEdge[];
}

const NODE_HEIGHT = 36;
const CHAR_WIDTH = 7.4;

let loading: Promise<ELK> | null = null;

function elk(): Promise<ELK> {
  loading ??= import('elkjs/lib/elk.bundled.js').then((module) => new module.default());
  return loading;
}

/** Lays a directed graph out left to right. Edges keep the order they were given in. */
export async function layoutGraph(
  nodes: Node[],
  edges: { from: string; to: string; label?: string }[],
): Promise<Layout> {
  const engine = await elk();

  const graph: ElkNode = {
    id: 'root',
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': 'RIGHT',
      'elk.spacing.nodeNode': '28',
      'elk.layered.spacing.nodeNodeBetweenLayers': '64',
      'elk.spacing.edgeLabel': '4',
      'elk.padding': '[top=14,left=14,bottom=14,right=14]',
    },
    children: nodes.map((n) => ({
      id: n.id,
      width: Math.max(72, n.label.length * CHAR_WIDTH + 28),
      height: NODE_HEIGHT,
    })),
    edges: edges.map((e, i) => ({
      id: `e${i}`,
      sources: [e.from],
      targets: [e.to],
      labels: e.label ? [{ text: e.label, width: e.label.length * 6.4 + 8, height: 16 }] : [],
    })),
  };

  const placed = await engine.layout(graph);
  const byID = new Map(nodes.map((n) => [n.id, n]));

  return {
    width: placed.width ?? 0,
    height: placed.height ?? 0,
    nodes: (placed.children ?? []).map((child) => ({
      id: child.id,
      label: byID.get(child.id)?.label ?? child.id,
      x: child.x ?? 0,
      y: child.y ?? 0,
      width: child.width ?? 0,
      height: child.height ?? 0,
    })),
    edges: (placed.edges ?? []).map((edge, i) => {
      const section = edge.sections?.[0];
      const points = section ? [section.startPoint, ...(section.bendPoints ?? []), section.endPoint] : [];
      const label = edge.labels?.[0];
      const middle = pointAt(points, 0.5);
      return {
        from: edges[i].from,
        to: edges[i].to,
        label: edges[i].label ?? '',
        points,
        at: label
          ? { x: (label.x ?? 0) + (label.width ?? 0) / 2, y: (label.y ?? 0) + (label.height ?? 0) / 2 }
          : middle,
      };
    }),
  };
}

/** The point a fraction of the way along a polyline, measured by length. */
export function pointAt(points: Point[], fraction: number): Point {
  if (points.length === 0) {
    return { x: 0, y: 0 };
  }
  const lengths = points.slice(1).map((p, i) => Math.hypot(p.x - points[i].x, p.y - points[i].y));
  const total = lengths.reduce((sum, n) => sum + n, 0);
  let remaining = Math.max(0, Math.min(1, fraction)) * total;

  for (let i = 0; i < lengths.length; i++) {
    if (remaining <= lengths[i] && lengths[i] > 0) {
      const t = remaining / lengths[i];
      return {
        x: points[i].x + (points[i + 1].x - points[i].x) * t,
        y: points[i].y + (points[i + 1].y - points[i].y) * t,
      };
    }
    remaining -= lengths[i];
  }
  return points[points.length - 1];
}

export function toPath(points: Point[]): string {
  return points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x},${p.y}`).join(' ');
}
