import { describe, expect, it } from 'vitest';
import { ahead, clauseWords, decisions, flagKind, pretty, segmentSize, variationLabel, variationTone } from './flags';
import type { Clause, Segment, Status, Variation } from './flags';

const bool: Variation[] = [
  { index: 0, name: '', description: '', value: 'true' },
  { index: 1, name: '', description: '', value: 'false' },
];

const json: Variation[] = [
  { index: 0, name: 'Classic', description: '', value: '{"layout":"classic"}' },
  { index: 1, name: '', description: '', value: '{"layout":"one-page","steps":["cart","details"]}' },
];

function status(over: Partial<Status>): Status {
  return {
    state: 'on',
    reason: '',
    name: '',
    description: '',
    project: 'default',
    environment: 'production',
    url: '',
    kind: 'boolean',
    environments: [],
    variations: bool,
    segments: [],
    targeting: null,
    ...over,
  };
}

function clause(over: Partial<Clause>): Clause {
  return { attribute: 'country', op: 'in', contextKind: 'user', values: [], negate: false, ...over };
}

describe('variationLabel', () => {
  it('prefers the name, then a short value, then a truncated one', () => {
    expect(variationLabel(json, 0)).toBe('Classic');
    expect(variationLabel(bool, 0)).toBe('true');
    expect(variationLabel(json, 1)).toBe('{"layout":"one-page","steps…');
  });

  it('copes with a variation that is not there', () => {
    expect(variationLabel(json, 7)).toBe('variation 8');
    expect(variationLabel(json, -1)).toBe('nothing');
    expect(variationLabel(null, 0)).toBe('variation 1');
  });
});

describe('variationTone', () => {
  it('keeps green for true whatever its index', () => {
    expect(variationTone(bool, 0)).toBe('v-true');
    expect(variationTone(bool, 1)).toBe('v-false');
    expect(variationTone([...bool].reverse(), 0)).toBe('v-false');
  });

  it('cycles through five colours otherwise', () => {
    expect(variationTone(json, 1)).toBe('v1');
    expect(variationTone(json, 6)).toBe('v1');
    expect(variationTone(json, -1)).toBe('v4');
  });
});

describe('flagKind', () => {
  it('names what a multivariate flag serves', () => {
    expect(flagKind(status({}))).toBe('boolean');
    expect(flagKind(status({ kind: 'multivariate', variations: json }))).toBe('JSON · 2 variations');
    expect(
      flagKind(
        status({ kind: 'multivariate', variations: [{ index: 0, name: '', description: '', value: '"blue"' }] }),
      ),
    ).toBe('string · 1 variations');
    expect(
      flagKind(status({ kind: 'multivariate', variations: [{ index: 0, name: '', description: '', value: '42' }] })),
    ).toBe('number · 1 variations');
  });
});

describe('clauseWords', () => {
  it('reads an ordinary clause, negated or not', () => {
    expect(clauseWords(clause({}))).toEqual({ subject: 'user country', verb: 'is', segment: false });
    expect(clauseWords(clause({ negate: true, contextKind: 'account', attribute: 'plan' }))).toEqual({
      subject: 'account plan',
      verb: 'is not',
      segment: false,
    });
    expect(clauseWords(clause({ op: 'semVerLessThan', attribute: 'appVersion', contextKind: 'device' })).verb).toBe(
      'is older than',
    );
  });

  it('reads a segment clause without its attribute', () => {
    expect(clauseWords(clause({ op: 'segmentMatch', attribute: 'segmentMatch', contextKind: '' }))).toEqual({
      subject: 'user',
      verb: 'is in segment',
      segment: true,
    });
    expect(clauseWords(clause({ op: 'segmentMatch', negate: true })).verb).toBe('is not in segment');
  });

  it('shows an operator it has no words for as it is', () => {
    expect(clauseWords(clause({ op: 'somethingNew' })).verb).toBe('somethingNew');
    expect(clauseWords(clause({ op: 'somethingNew', negate: true })).verb).toBe('not somethingNew');
  });
});

describe('pretty', () => {
  it('lays out objects and arrays, and leaves the rest alone', () => {
    expect(pretty('{"a":1}')).toEqual({ text: '{\n  "a": 1\n}', line: '{"a":1}', block: true });
    expect(pretty('[1,2]').block).toBe(true);
    expect(pretty('"blue"')).toEqual({ text: '"blue"', line: '"blue"', block: false });
    expect(pretty('null').block).toBe(false);
    expect(pretty('not json')).toEqual({ text: 'not json', line: 'not json', block: false });
  });
});

describe('segmentSize', () => {
  const segment: Segment = { key: 's', name: '', description: '', rules: [], included: 0, excluded: 0, found: true };

  it('says who is in it', () => {
    expect(segmentSize({ ...segment, included: 18, excluded: 2 })).toBe('18 listed by name, 2 excluded');
    expect(segmentSize({ ...segment, rules: [[clause({})]] })).toBe('1 rule');
    expect(segmentSize(segment)).toBe('empty');
  });
});

describe('ahead and decisions', () => {
  it('counts what is decided before the default', () => {
    const rule = { description: '', clauses: [], serve: { variation: 0, rollout: [] } };
    const s = status({
      targeting: {
        prerequisites: [{ key: 'other', variation: 0 }],
        targets: [
          { contextKind: 'user', variation: 0, count: 2 },
          { contextKind: 'account', variation: 1, count: 1 },
        ],
        rules: [rule, rule],
        fallthrough: { variation: 1, rollout: [] },
        offVariation: 1,
      },
    });

    expect(ahead(s)).toBe('1 prerequisite, 3 named targets, 2 rules');
    expect(decisions(s)).toBe(5);
  });

  it('is empty when the default is all there is, or nothing is known', () => {
    const plain = status({
      targeting: {
        prerequisites: [],
        targets: [],
        rules: [],
        fallthrough: { variation: 0, rollout: [] },
        offVariation: 1,
      },
    });

    expect(ahead(plain)).toBe('');
    expect(decisions(plain)).toBe(0);
    expect(ahead(status({}))).toBe('');
  });
});
