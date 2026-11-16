import type { Location } from './api';

export type Tone = 'ok' | 'amber' | 'red';

/** How loudly a status should be shown. Anything unrecognized is treated as a warning. */
export function toneOf(status: string): Tone {
  switch (status) {
    case 'ok':
    case '':
      return 'ok';
    case 'missing':
      return 'red';
    default:
      return 'amber';
  }
}

/** The short badge text for an anchor that didn't resolve cleanly. */
export function statusLabel(location: Location): string {
  switch (location.status as string) {
    case 'moved': {
      const lines = Math.abs(location.offset) === 1 ? 'line' : 'lines';
      return `${location.offset > 0 ? '+' : '−'}${Math.abs(location.offset)} ${lines}`;
    }
    case 'ambiguous':
      return `${location.matches} matches`;
    case 'changed':
      return 'Code changed';
    case 'missing':
      return 'Missing';
    case 'unverified':
      return "Can't verify";
    default:
      return '';
  }
}

/** How long ago something happened, in the largest unit that fits: "9 months". */
export function ago(when: string | Date, now = new Date()): string {
  const then = new Date(when);
  // Go's zero time means "unknown".
  if (Number.isNaN(then.getTime()) || then.getFullYear() < 1971) {
    return '';
  }
  const days = Math.max(0, Math.floor((now.getTime() - then.getTime()) / 86_400_000));
  const count = (n: number, unit: string) => `${n} ${unit}${n === 1 ? '' : 's'}`;
  if (days < 1) {
    return 'less than a day';
  }
  if (days < 60) {
    return count(days, 'day');
  }
  if (days < 730) {
    return count(Math.floor(days / 30), 'month');
  }
  return count(Math.floor(days / 365), 'year');
}
