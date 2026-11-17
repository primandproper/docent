import { describe, expect, it } from 'vitest';
import { findCall } from './calls';
import type { Excerpt } from './calls';

function excerpt(text: string, startLine = 1, endLine = text.split('\n').length): Excerpt {
  return { lines: text.split('\n'), startLine, endLine };
}

describe('findCall', () => {
  it('joins a call to the function the next step highlights', () => {
    const caller = excerpt('func whatever() {\n\tstuff()\n}');
    const callee = excerpt('package x\n\n// stuff does things.\nfunc stuff() {\n}', 3, 5);

    expect(findCall(caller, callee)).toEqual({
      name: 'stuff',
      call: { line: 2, col: 1 },
      definition: { line: 4, col: 5 },
    });
  });

  it('follows a method call to a method with a receiver', () => {
    const caller = excerpt(
      'func (h *Handler) List(w http.ResponseWriter) {\n\tif err := h.respond(w, items); err != nil {\n\t}\n}',
    );
    const callee = excerpt('func (h *Handler) respond(w http.ResponseWriter, value any) error {');

    expect(findCall(caller, callee)).toMatchObject({ name: 'respond', call: { line: 2, col: 13 } });
  });

  it('reads other ways of writing a function', () => {
    const caller = excerpt('  await sleep(100);\n  const out = parse<Item>(raw);\n  retry(3)');

    expect(findCall(caller, excerpt('function sleep(ms: number): Promise<void> {'))?.name).toBe('sleep');
    expect(findCall(caller, excerpt('export async function sleep(ms) {'))?.name).toBe('sleep');
    expect(findCall(caller, excerpt('const sleep = (ms: number) => new Promise(() => ms);'))?.name).toBe('sleep');
    expect(findCall(caller, excerpt('export function parse<T>(raw: string): T {'))?.name).toBe('parse');
    expect(findCall(caller, excerpt('@cached\ndef retry(times):'))?.name).toBe('retry');
    expect(findCall(excerpt('item = Item(name)'), excerpt('class Item:'))?.name).toBe('Item');
  });

  it('looks only inside the highlights', () => {
    const caller = excerpt('stuff()\nfunc whatever() {\n\tother()\n}', 2, 4);
    const callee = excerpt('func stuff() {\n}\n\nfunc other() {\n}', 1, 2);

    expect(findCall(caller, callee)).toBeNull();
  });

  it('ignores a mention in a comment, and a longer name that ends the same', () => {
    expect(findCall(excerpt('// stuff() is called later\nfoodstuff()'), excerpt('func stuff() {'))).toBeNull();
  });

  it('finds nothing when either step has no highlight', () => {
    expect(findCall(excerpt('stuff()', 0, 0), excerpt('func stuff() {'))).toBeNull();
    expect(findCall(excerpt('stuff()'), excerpt('func stuff() {', 0, 0))).toBeNull();
  });
});
