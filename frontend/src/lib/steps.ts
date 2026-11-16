import type { Step } from './api';

export function basename(path: string): string {
  const parts = path.replaceAll('\\', '/').split('/').filter(Boolean);
  return parts[parts.length - 1] ?? path;
}

/** Strips the Markdown that would look like noise in a one-line label. */
function plain(markdown: string): string {
  return markdown
    .replace(/^#+\s*/, '')
    .replace(/[*_`]/g, '')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .trim();
}

/** A step's label for the rail: its title, else its file, else how its text begins. */
export function stepTitle(step: Step): string {
  if (step.title) {
    return step.title;
  }
  const target = step.file || step.directory;
  if (target) {
    return basename(target);
  }
  const firstLine = step.description.split('\n').find((line) => line.trim() !== '') ?? '';
  return plain(firstLine) || step.uri || 'Untitled step';
}

/** Parses what someone typed into "go to step": a 1-based number within range. */
export function parseStepNumber(input: string, count: number): number | null {
  if (!/^\d+$/.test(input.trim())) {
    return null;
  }
  const n = Number(input.trim());
  return n >= 1 && n <= count ? n - 1 : null;
}

/**
 * Where to land after a tour is reloaded: the step with the same id if it
 * still exists, else the same index, clamped to the new length.
 */
export function positionAfterReload(previous: { id: string; index: number }, steps: Step[]): number {
  if (previous.id) {
    const byID = steps.findIndex((s) => s.id === previous.id);
    if (byID >= 0) {
      return byID;
    }
  }
  return Math.max(0, Math.min(previous.index, steps.length - 1));
}
