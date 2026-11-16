import { describe, expect, it } from 'vitest';
import { languageFor, tokenize } from './highlight';

describe('languageFor', () => {
  it('maps extensions to grammars', () => {
    expect(languageFor('internal/http/items.go')).toBe('go');
    expect(languageFor('src/App.svelte')).toBe('svelte');
    expect(languageFor('a/b/c.yml')).toBe('yaml');
    expect(languageFor('x.TS')).toBe('ts');
  });

  it('knows files by name', () => {
    expect(languageFor('build/Dockerfile')).toBe('docker');
    expect(languageFor('Makefile')).toBe('make');
    expect(languageFor('Dockerfile.dev')).toBe('docker');
  });

  it('returns null without a grammar', () => {
    expect(languageFor('LICENSE')).toBeNull();
    expect(languageFor('data.unknownext')).toBeNull();
  });
});

describe('tokenize', () => {
  it('returns a row of tokens per line, in both themes', async () => {
    const lines = ['package main', '', 'func main() {}'];
    const tokens = await tokenize('main.go', lines);

    expect(tokens).not.toBeNull();
    expect(tokens).toHaveLength(3);
    expect(tokens?.[0].map((t) => t.text).join('')).toBe('package main');
    expect(tokens?.[0][0].light).toMatch(/^#/);
    expect(tokens?.[0][0].dark).toMatch(/^#/);
  });

  it('returns null for an unknown language', async () => {
    expect(await tokenize('notes.unknownext', ['x'])).toBeNull();
  });
});
