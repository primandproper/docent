// Words and colours for a flag card. The status itself comes from Go: nothing
// here decides anything about a flag, it only says it.
import type {
  Clause,
  Segment,
  Serve,
  Status,
  Variation,
} from '../../bindings/github.com/primandproper/docent/internal/flags';

export type {
  Clause,
  Segment,
  Serve,
  Status,
  Targeting,
  Variation,
} from '../../bindings/github.com/primandproper/docent/internal/flags';

/** Serving one variation and nothing else. */
export function only(variation: number): Serve {
  return { variation, rollout: [] };
}

/** What to call a variation: its name, or its value when that is short enough to read. */
export function variationLabel(variations: Variation[] | null, index: number): string {
  const v = variations?.[index];
  if (!v) {
    return index < 0 ? 'nothing' : `variation ${index + 1}`;
  }
  return v.name || (v.value.length > 28 ? `${v.value.slice(0, 27)}…` : v.value);
}

/** A colour class per variation. A boolean's are fixed, so green always means true. */
export function variationTone(variations: Variation[] | null, index: number): string {
  const value = variations?.[index]?.value;
  if (value === 'true') {
    return 'v-true';
  }
  if (value === 'false') {
    return 'v-false';
  }
  return `v${((index % 5) + 5) % 5}`;
}

/** `boolean`, or what kind of value a multivariate flag serves and how many there are. */
export function flagKind(status: Status): string {
  if (status.kind === 'boolean') {
    return 'boolean';
  }
  const first = status.variations?.[0]?.value ?? '';
  const kind = first.startsWith('{') || first.startsWith('[') ? 'JSON' : first.startsWith('"') ? 'string' : 'number';
  return `${kind} · ${status.variations?.length ?? 0} variations`;
}

const OPS: Record<string, [string, string]> = {
  in: ['is', 'is not'],
  endsWith: ['ends with', 'does not end with'],
  startsWith: ['starts with', 'does not start with'],
  matches: ['matches', 'does not match'],
  contains: ['contains', 'does not contain'],
  lessThan: ['<', 'is not <'],
  lessThanOrEqual: ['≤', 'is not ≤'],
  greaterThan: ['>', 'is not >'],
  greaterThanOrEqual: ['≥', 'is not ≥'],
  before: ['is before', 'is not before'],
  after: ['is after', 'is not after'],
  semVerEqual: ['is version', 'is not version'],
  semVerLessThan: ['is older than', 'is not older than'],
  semVerGreaterThan: ['is newer than', 'is not newer than'],
};

/** A clause as the words before its values: "account plan is not", "user is in segment". */
export function clauseWords(clause: Clause): { subject: string; verb: string; segment: boolean } {
  const kind = clause.contextKind || 'user';
  if (clause.op === 'segmentMatch') {
    return { subject: kind, verb: clause.negate ? 'is not in segment' : 'is in segment', segment: true };
  }
  const [yes, no] = OPS[clause.op] ?? [clause.op, `not ${clause.op}`];
  return { subject: `${kind} ${clause.attribute}`, verb: clause.negate ? no : yes, segment: false };
}

/** A variation's value for reading: an object or array laid out, anything else as it is. */
export function pretty(value: string): { text: string; line: string; block: boolean } {
  try {
    const parsed: unknown = JSON.parse(value);
    if (parsed !== null && typeof parsed === 'object') {
      return { text: JSON.stringify(parsed, null, 2), line: JSON.stringify(parsed), block: true };
    }
  } catch {
    // Shown as it came.
  }
  return { text: value, line: value, block: false };
}

/** Who is in a segment, in a few words: "18 listed by name, 2 excluded". */
export function segmentSize(segment: Segment): string {
  const rules = segment.rules?.length ?? 0;
  return (
    [
      segment.included > 0 ? `${segment.included} listed by name` : '',
      rules > 0 ? `${rules} rule${rules > 1 ? 's' : ''}` : '',
      segment.excluded > 0 ? `${segment.excluded} excluded` : '',
    ]
      .filter(Boolean)
      .join(', ') || 'empty'
  );
}

function count(n: number, noun: string): string {
  return n > 0 ? `${n} ${noun}${n > 1 ? 's' : ''}` : '';
}

/** What is decided before the default: "1 prerequisite, 3 named targets, 2 rules", or "" when nothing is. */
export function ahead(status: Status): string {
  const t = status.targeting;
  if (!t) {
    return '';
  }
  const named = (t.targets ?? []).reduce((sum, target) => sum + target.count, 0);
  return [
    count(t.prerequisites?.length ?? 0, 'prerequisite'),
    count(named, 'named target'),
    count(t.rules?.length ?? 0, 'rule'),
  ]
    .filter(Boolean)
    .join(', ');
}

/** How many lines the decision list has, not counting the default. */
export function decisions(status: Status): number {
  const t = status.targeting;
  return t ? (t.prerequisites?.length ?? 0) + (t.targets?.length ?? 0) + (t.rules?.length ?? 0) : 0;
}
