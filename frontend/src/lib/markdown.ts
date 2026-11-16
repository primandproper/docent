import DOMPurify from 'dompurify';
import MarkdownIt from 'markdown-it';

// Raw HTML in the source is escaped, not rendered: tours are untrusted input.
const md = new MarkdownIt({ html: false, linkify: true, typographer: false });

const purifier = DOMPurify(window);

purifier.addHook('afterSanitizeAttributes', (node) => {
  // The app never fetches anything a tour names, so a remote image becomes its
  // alt text. Embedded data: images are fine.
  if (node instanceof HTMLImageElement && !node.getAttribute('src')?.startsWith('data:image/')) {
    node.replaceWith(document.createTextNode(node.alt ? `[image: ${node.alt}]` : '[image]'));
  }
});

/** Renders untrusted Markdown to HTML that is safe to put in the page. */
export function renderMarkdown(source: string): string {
  return purifier.sanitize(md.render(source), {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ['style', 'form', 'input', 'button', 'iframe', 'video', 'audio', 'svg', 'math'],
    FORBID_ATTR: ['style', 'srcset'],
    // Docent's own links are allowed alongside the web's. Nothing else is.
    ALLOWED_URI_REGEXP: /^(?:https?:|mailto:|docent:(?:step|tour)\/|#)/i,
  });
}
