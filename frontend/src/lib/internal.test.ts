import { describe, expect, it } from 'vitest';
import { parseInternal } from './internal';

describe('parseInternal', () => {
  it('reads step and tour links', () => {
    expect(parseInternal('docent:step/write-path')).toEqual({ kind: 'step', id: 'write-path' });
    expect(parseInternal('docent:tour/sub/other.tour#start')).toEqual({
      kind: 'tour',
      path: 'sub/other.tour',
      id: 'start',
    });
    expect(parseInternal('docent:tour/other.tour')).toEqual({ kind: 'tour', path: 'other.tour', id: '' });
  });

  it('rejects everything else', () => {
    for (const href of ['docent:', 'docent:step/', 'docent:tour/', 'docent:shell/x', 'https://example.com', '']) {
      expect(parseInternal(href)).toBeNull();
    }
  });
});
