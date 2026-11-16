import { describe, expect, it } from 'vitest';
import { extractClicks } from './diagram';

describe('extractClicks', () => {
  it('keeps docent links for wiring up and drops every click from the source', () => {
    const { source, clicks } = extractClicks(
      [
        'flowchart LR',
        '  api --> queue',
        '  click api "docent:step/write-path"',
        '  click queue href "docent:tour/other.tour#start" "tooltip"',
        '  click api call alert()',
        '  click queue "https://example.com"',
      ].join('\n'),
    );

    expect(source).toBe('flowchart LR\n  api --> queue');
    expect(clicks).toEqual([
      { node: 'api', href: 'docent:step/write-path' },
      { node: 'queue', href: 'docent:tour/other.tour#start' },
    ]);
  });
});
