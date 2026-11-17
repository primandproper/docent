// Finding the call that joins two steps: one highlights code that calls a
// function, the next highlights that function. It is a reading of the text,
// not of the language, so it works wherever a call is written `name(`.

export interface Excerpt {
  /** The whole file, a line per entry. */
  lines: string[];
  /** The highlighted lines, 1-based and inclusive. 0 for no highlight. */
  startLine: number;
  endLine: number;
}

export interface Spot {
  /** 1-based. */
  line: number;
  /** 0-based index into the line's text. */
  col: number;
}

export interface Call {
  name: string;
  /** The name where it is called. */
  call: Spot;
  /** The name where it is defined. */
  definition: Spot;
}

/** Words that sit before a parenthesis without being a function's name. */
const NOT_NAMES = new Set([
  'async',
  'await',
  'catch',
  'def',
  'defer',
  'else',
  'fn',
  'for',
  'func',
  'function',
  'go',
  'if',
  'in',
  'match',
  'new',
  'return',
  'switch',
  'throw',
  'typeof',
  'while',
  'with',
  'yield',
]);

const COMMENT = /^\s*(\/\/|\/\*|\*|#|--|"""|'''|@|<!--)/;
const NAME_BEFORE_PAREN = /([A-Za-z_$][\w$]*)\s*(?:<[^<>()]*>)?\(/g;
const NAME_BEFORE_ASSIGN = /([A-Za-z_$][\w$]*)\s*(?::[^=]*)?=(?!=)/g;
const NAME_AFTER_KEYWORD = /\b(?:class|struct|type|interface|trait|enum)\s+([A-Za-z_$][\w$]*)/g;

function names(pattern: RegExp, text: string): { name: string; col: number }[] {
  return [...text.matchAll(pattern)]
    .map((match) => ({ name: match[1], col: match.index + match[0].indexOf(match[1]) }))
    .filter((found) => !NOT_NAMES.has(found.name));
}

/** The names the first line of code in an excerpt's highlight could be defining, likeliest first. */
function definitions(excerpt: Excerpt): { name: string; spot: Spot }[] {
  for (let line = excerpt.startLine; line <= excerpt.endLine; line++) {
    const text = excerpt.lines[line - 1] ?? '';
    if (text.trim() === '' || COMMENT.test(text)) {
      continue;
    }
    return [
      ...names(NAME_BEFORE_PAREN, text),
      ...names(NAME_AFTER_KEYWORD, text),
      ...names(NAME_BEFORE_ASSIGN, text),
    ].map((found) => ({ name: found.name, spot: { line, col: found.col } }));
  }
  return [];
}

function escape(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

/**
 * The call in `caller`'s highlight to the function `callee`'s highlight
 * starts with, or null when there isn't one.
 */
export function findCall(caller: Excerpt, callee: Excerpt): Call | null {
  if (caller.startLine <= 0 || callee.startLine <= 0) {
    return null;
  }

  for (const definition of definitions(callee)) {
    const called = new RegExp(`(?<![\\w$])${escape(definition.name)}\\s*(?:<[^<>()]*>)?\\(`);
    for (let line = caller.startLine; line <= caller.endLine; line++) {
      const text = caller.lines[line - 1] ?? '';
      if (COMMENT.test(text)) {
        continue;
      }
      const match = called.exec(text);
      if (match) {
        return { name: definition.name, call: { line, col: match.index }, definition: definition.spot };
      }
    }
  }
  return null;
}
