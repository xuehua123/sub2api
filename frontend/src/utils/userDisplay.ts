type SegmenterConstructor = new (
  locale?: string,
  options?: { granularity?: 'grapheme' | 'word' | 'sentence' },
) => { segment(value: string): Iterable<{ segment: string }> }

function splitDisplayGraphemes(value: string): string[] {
  const Segmenter = (Intl as unknown as { Segmenter?: SegmenterConstructor }).Segmenter
  if (!Segmenter) return Array.from(value)

  return Array.from(new Segmenter(undefined, { granularity: 'grapheme' }).segment(value), (item) => item.segment)
}

export function maskEmailForDisplay(email?: string | null): string {
  const normalized = email?.trim().toLowerCase() ?? ''
  if (!normalized) return ''

  const [local, domain] = normalized.split('@')
  if (!local || !domain) return normalized

  const chars = splitDisplayGraphemes(local)
  if (chars.length <= 4) {
    return `${chars[0]}***@${domain}`
  }

  const maskLength = 3
  const headLength = Math.max(1, Math.floor((chars.length - maskLength + 1) / 2))
  const tailLength = Math.max(1, chars.length - maskLength - headLength)

  return `${chars.slice(0, headLength).join('')}***${chars.slice(-tailLength).join('')}@${domain}`
}

export function userDisplayName(username?: string | null, email?: string | null, fallback = ''): string {
  const normalizedUsername = username?.trim()
  if (normalizedUsername) return normalizedUsername

  return maskEmailForDisplay(email) || fallback
}

export function leadingGraphemes(value: string, count: number): string {
  return splitDisplayGraphemes(value).slice(0, count).join('')
}
