import DOMPurify from 'dompurify'
import { Marked, Renderer } from 'marked'

const renderer = new Renderer()

renderer.html = ({ text }) => escapeHtml(text)
renderer.image = ({ text }) => escapeHtml(text)

const marked = new Marked({
  async: false,
  breaks: true,
  gfm: true,
  renderer,
})

const sanitizeConfig = {
  ALLOWED_TAGS: [
    'a',
    'blockquote',
    'br',
    'code',
    'del',
    'em',
    'h1',
    'h2',
    'h3',
    'h4',
    'h5',
    'h6',
    'hr',
    'li',
    'ol',
    'p',
    'pre',
    'strong',
    'table',
    'tbody',
    'td',
    'th',
    'thead',
    'tr',
    'ul',
  ],
  ALLOWED_ATTR: ['href', 'title'],
  ALLOW_DATA_ATTR: false,
  FORBID_TAGS: ['button', 'embed', 'form', 'iframe', 'img', 'input', 'math', 'object', 'script', 'style', 'svg'],
}

export function renderIssueMarkdown(content?: string | null): string {
  const source = content?.trim()
  if (!source) return ''

  const html = marked.parse(source, { async: false })
  return DOMPurify.sanitize(html, sanitizeConfig)
}

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (character) => {
    switch (character) {
      case '&':
        return '&amp;'
      case '<':
        return '&lt;'
      case '>':
        return '&gt;'
      case '"':
        return '&quot;'
      case "'":
        return '&#39;'
      default:
        return character
    }
  })
}
