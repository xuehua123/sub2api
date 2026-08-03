export function hasUnsafeConnectivityURLSyntax(rawURL: string): boolean {
  const trimmed = rawURL.trim()
  if (trimmed.includes('\\') || trimmed.includes('%')) return true
  for (const character of trimmed) {
    const codePoint = character.codePointAt(0) ?? 0
    if (codePoint <= 0x20 || codePoint > 0x7e) return true
  }

  const authorityStart = trimmed.indexOf('://')
  if (authorityStart < 0) return false
  const pathStart = trimmed.indexOf('/', authorityStart + 3)
  if (pathStart < 0) return false
  const suffix = trimmed.slice(pathStart)
  const pathEnd = suffix.search(/[?#]/)
  const rawPath = pathEnd < 0 ? suffix : suffix.slice(0, pathEnd)
  return rawPath.split('/').some((segment) => segment === '.' || segment === '..')
}
