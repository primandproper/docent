import { createHighlighterCore } from 'shiki/core';
import type { HighlighterCore } from 'shiki/core';
import { createJavaScriptRegexEngine } from 'shiki/engine-javascript.mjs';
import { bundledLanguages, bundledLanguagesInfo } from 'shiki/langs';
import { basename } from './steps';

/** One run of same-colored text: its light and dark colors, and font style bits. */
export interface Token {
  text: string;
  light: string;
  dark: string;
  italic: boolean;
  bold: boolean;
}

const LIGHT = 'github-light-default';
const DARK = 'github-dark-default';

// Files whose language the extension doesn't give away.
const BY_NAME: Record<string, string> = {
  'dockerfile': 'docker',
  'makefile': 'make',
  'gnumakefile': 'make',
  'cmakelists.txt': 'cmake',
  'go.mod': 'go',
  'jenkinsfile': 'groovy',
  '.bashrc': 'bash',
  '.zshrc': 'bash',
  'gemfile': 'ruby',
  'rakefile': 'ruby',
};

const BY_EXTENSION: Record<string, string> = {
  mts: 'ts',
  cts: 'ts',
  mjs: 'js',
  cjs: 'js',
  h: 'c',
  hpp: 'cpp',
  cc: 'cpp',
  hh: 'cpp',
  yml: 'yaml',
  htm: 'html',
  tf: 'terraform',
  mmd: 'mermaid',
  plist: 'xml',
  tour: 'json',
};

const known = new Set<string>();
for (const info of bundledLanguagesInfo) {
  known.add(info.id);
  for (const alias of info.aliases ?? []) {
    known.add(alias);
  }
}

/** The Shiki language for a file path, or null when there's no grammar for it. */
export function languageFor(path: string): string | null {
  const name = basename(path).toLowerCase();
  if (BY_NAME[name]) {
    return BY_NAME[name];
  }
  if (name.startsWith('dockerfile.') || name.endsWith('.dockerfile')) {
    return 'docker';
  }
  const dot = name.lastIndexOf('.');
  if (dot < 0) {
    return null;
  }
  const ext = name.slice(dot + 1);
  const lang = BY_EXTENSION[ext] ?? ext;
  return known.has(lang) ? lang : null;
}

let highlighterPromise: Promise<HighlighterCore> | null = null;

function highlighter(): Promise<HighlighterCore> {
  // The JavaScript regex engine keeps WebAssembly out of the webview, so the
  // content security policy doesn't need to allow it.
  highlighterPromise ??= createHighlighterCore({
    themes: [import('shiki/themes/github-light-default.mjs'), import('shiki/themes/github-dark-default.mjs')],
    langs: [],
    engine: createJavaScriptRegexEngine({ forgiving: true }),
  });
  return highlighterPromise;
}

/**
 * Tokenizes lines of a file. Grammars load on first use, one language at a
 * time. Returns null when the language is unknown or its grammar fails, and
 * the caller shows plain text.
 */
export async function tokenize(path: string, lines: string[]): Promise<Token[][] | null> {
  const lang = languageFor(path);
  if (!lang) {
    return null;
  }

  try {
    const shiki = await highlighter();
    if (!shiki.getLoadedLanguages().includes(lang)) {
      const load = bundledLanguages[lang as keyof typeof bundledLanguages];
      if (!load) {
        return null;
      }
      await shiki.loadLanguage(load);
    }

    const result = shiki.codeToTokensWithThemes(lines.join('\n'), {
      lang,
      themes: { light: LIGHT, dark: DARK },
    });

    return result.map((line) =>
      line.map((token) => {
        const fontStyle = token.variants.light?.fontStyle ?? 0;
        return {
          text: token.content,
          light: token.variants.light?.color ?? '',
          dark: token.variants.dark?.color ?? '',
          italic: (fontStyle & 1) !== 0,
          bold: (fontStyle & 2) !== 0,
        };
      }),
    );
  } catch (err) {
    console.warn(`highlighting ${path} as ${lang} failed`, err);
    return null;
  }
}
