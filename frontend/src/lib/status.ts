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
