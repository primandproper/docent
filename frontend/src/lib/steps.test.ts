import { describe, expect, it } from 'vitest';
import type { Step } from './api';
import { parseStepNumber, positionAfterReload, stepTitle } from './steps';

function step(overrides: Partial<Step>): Step {
  return { title: '', description: '', file: '', directory: '', uri: '', id: '', ...overrides } as Step;
}

describe('stepTitle', () => {
  it('prefers the title', () => {
    expect(stepTitle(step({ title: 'The handler', file: 'a/b.go' }))).toBe('The handler');
  });

  it('falls back to the file or directory name', () => {
    expect(stepTitle(step({ file: 'internal/http/items.go' }))).toBe('items.go');
    expect(stepTitle(step({ directory: 'internal/http/' }))).toBe('http');
    expect(stepTitle(step({ file: 'src\\win\\a.ts' }))).toBe('a.ts');
  });

  it('falls back to the first line of the description, without its markup', () => {
    expect(stepTitle(step({ description: '\n## Welcome to **the** [tour](x)\nmore' }))).toBe('Welcome to the tour');
  });

  it('never returns an empty label', () => {
    expect(stepTitle(step({}))).toBe('Untitled step');
  });
});

describe('parseStepNumber', () => {
  it('accepts numbers in range, 1-based', () => {
    expect(parseStepNumber('1', 5)).toBe(0);
    expect(parseStepNumber(' 5 ', 5)).toBe(4);
  });

  it('rejects everything else', () => {
    expect(parseStepNumber('0', 5)).toBeNull();
    expect(parseStepNumber('6', 5)).toBeNull();
    expect(parseStepNumber('2a', 5)).toBeNull();
    expect(parseStepNumber('', 5)).toBeNull();
  });
});

describe('positionAfterReload', () => {
  const steps = [step({ id: 'a' }), step({ id: 'b' }), step({ id: 'c' })];

  it('follows the step id when it moved', () => {
    expect(positionAfterReload({ id: 'c', index: 0 }, steps)).toBe(2);
  });

  it('keeps the index when there is no id or it is gone', () => {
    expect(positionAfterReload({ id: '', index: 1 }, steps)).toBe(1);
    expect(positionAfterReload({ id: 'gone', index: 1 }, steps)).toBe(1);
  });

  it('clamps to the new length', () => {
    expect(positionAfterReload({ id: '', index: 9 }, steps)).toBe(2);
    expect(positionAfterReload({ id: '', index: 3 }, [])).toBe(0);
  });
});
