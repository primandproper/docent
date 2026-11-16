/** One of Docent's own links: to a step of this tour, or into another tour. */
export type InternalLink = { kind: 'step'; id: string } | { kind: 'tour'; path: string; id: string };

/** Parses `docent:step/<id>` and `docent:tour/<path>#<id>`. Anything else is null. */
export function parseInternal(href: string): InternalLink | null {
  if (href.startsWith('docent:step/')) {
    const id = href.slice('docent:step/'.length);
    return id ? { kind: 'step', id } : null;
  }
  if (href.startsWith('docent:tour/')) {
    const [path, id = ''] = href.slice('docent:tour/'.length).split('#');
    return path ? { kind: 'tour', path, id } : null;
  }
  return null;
}
