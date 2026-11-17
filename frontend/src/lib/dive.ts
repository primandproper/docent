// The animation for a step that follows a call (`"transition": "call"`): the
// function's body opens out of the line that calls it, and going back, folds
// into it again. It is drawn with the two code panes themselves: a copy of the
// one being left, and the live one being arrived at.
import type { Call, Spot } from './calls';

const DURATION_MS = 560;
const EASING = 'cubic-bezier(0.4, 0, 0.2, 1)';

interface Found {
  /** The name itself. */
  name: DOMRect;
  /** The whole line it is on. */
  row: DOMRect;
}

/** Where a name is on screen in a code pane, or null if its line isn't rendered or isn't wholly in view. */
function locate(pane: HTMLElement, spot: Spot, length: number): Found | null {
  const row = pane.querySelector(`.row[data-line="${spot.line}"]`);
  const text = row?.querySelector('.text');
  if (!row || !text) {
    return null;
  }

  // The line is one text node before it is highlighted and many after.
  const range = document.createRange();
  const walker = document.createTreeWalker(text, NodeFilter.SHOW_TEXT);
  let offset = 0;
  let started = false;
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const size = node.textContent?.length ?? 0;
    if (!started && spot.col < offset + size) {
      range.setStart(node, spot.col - offset);
      started = true;
    }
    if (started && spot.col + length <= offset + size) {
      range.setEnd(node, spot.col + length - offset);
      break;
    }
    offset += size;
  }
  if (!started || range.collapsed) {
    return null;
  }

  const view = pane.getBoundingClientRect();
  const found = { name: range.getBoundingClientRect(), row: row.getBoundingClientRect() };
  const inView =
    found.row.top >= view.top - 1 &&
    found.row.bottom <= view.bottom + 1 &&
    found.name.left >= view.left &&
    found.name.right <= view.right;
  return inView ? found : null;
}

export interface CallTransition {
  /** The positioned box both panes share. It clips what moves outside it. */
  host: HTMLElement;
  /** The pane now showing. */
  pane: HTMLElement;
  /** A copy of the pane that was showing, with where it was scrolled to. */
  copy: HTMLElement;
  scrollTop: number;
  scrollLeft: number;
  call: Call;
  /** in: from the caller to the function. out: back from the function to its caller. */
  direction: 'in' | 'out';
}

/**
 * Plays the transition. Returns a function that ends it early, or null when
 * it didn't play: the call or the definition wasn't in view, or the reader
 * asked for less motion.
 */
export function playCall({
  host,
  pane,
  copy,
  scrollTop,
  scrollLeft,
  call,
  direction,
}: CallTransition): (() => void) | null {
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    return null;
  }

  copy.classList.add('dive-copy');
  copy.setAttribute('aria-hidden', 'true');
  copy.inert = true;
  // The host has to be what the copy is laid out against before anything is measured.
  host.classList.add('diving');
  host.append(copy);
  copy.scrollTop = scrollTop;
  copy.scrollLeft = scrollLeft;

  const [caller, callee] = direction === 'in' ? [copy, pane] : [pane, copy];
  const from = locate(caller, call.call, call.name.length);
  const to = locate(callee, call.definition, call.name.length);
  if (!from || !to) {
    copy.remove();
    host.classList.remove('diving');
    return null;
  }

  const box = host.getBoundingClientRect();
  // The function starts as one line, its first, lying on the line that calls it.
  const shift = from.row.top - to.row.top;
  const above = to.row.top - box.top;
  const below = box.bottom - to.row.bottom;
  const folded = { clipPath: `inset(${above}px 0 ${below}px 0)`, transform: `translateY(${shift}px)` };
  const open = { clipPath: 'inset(0 0 0 0)', transform: 'translateY(0)' };

  const name = document.createElement('span');
  name.className = 'dive-name';
  name.textContent = call.name;
  name.setAttribute('aria-hidden', 'true');
  name.style.left = `${to.name.left - box.left}px`;
  name.style.top = `${to.name.top - box.top}px`;
  name.style.height = `${to.name.height}px`;
  host.append(name);
  // The name rides from where it is called to where it is defined.
  const atCall = { transform: `translate(${from.name.left - to.name.left}px, ${from.name.top - to.name.top}px)` };
  const atDefinition = { transform: 'translate(0, 0)' };

  callee.classList.add('dive-callee');

  const timing = { duration: DURATION_MS, easing: EASING, fill: 'both' as const };
  // The name keeps exactly the pane's pace, so it stays on the line it belongs to, and goes once it has landed.
  const fade = { duration: DURATION_MS * 0.25, delay: DURATION_MS * 0.75, fill: 'both' as const };
  const animations =
    direction === 'in'
      ? [
          callee.animate([folded, open], timing),
          caller.animate([{ opacity: 1 }, { opacity: 0.25 }], timing),
          name.animate([atCall, atDefinition], timing),
          name.animate([{ opacity: 1 }, { opacity: 0 }], fade),
        ]
      : [
          callee.animate([open, folded], timing),
          caller.animate([{ opacity: 0.25 }, { opacity: 1 }], timing),
          name.animate([atDefinition, atCall], timing),
          name.animate([{ opacity: 1 }, { opacity: 0 }], fade),
        ];

  let ended = false;
  const end = () => {
    if (ended) {
      return;
    }
    ended = true;
    animations.forEach((animation) => animation.cancel());
    copy.remove();
    name.remove();
    host.classList.remove('diving');
    callee.classList.remove('dive-callee');
  };
  animations[0].finished.then(end, end);
  return end;
}
