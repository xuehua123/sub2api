// Preserve installations below a path prefix, but never open credentials,
// query tokens or a non-HTTP protocol from persisted upstream configuration.
export function upstreamWebsite(raw: string): string {
  try {
    const url = new URL(raw)
    if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) return ''
    url.search = ''
    url.hash = ''
    url.pathname = url.pathname.replace(/\/(?:api(?:\/v1)?|v1)\/?$/, '') || '/'
    return url.toString()
  } catch {
    return ''
  }
}
