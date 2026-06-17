import type { OpsAccountHealthItem } from '@/api/admin/ops'

export type TagMatchMode = 'any' | 'all'

export const DEFAULT_TAG_OPTIONS = ['pro', 'plus', '生图', 'ccmax']
export const TAG_MAX_COUNT = 20
export const TAG_MAX_LENGTH = 32

export function normalizeTags(raw: unknown): string[] {
  const source = Array.isArray(raw)
    ? raw
    : typeof raw === 'string'
      ? raw.split(/[,，;；\n\t]/)
      : []
  const tags: string[] = []
  const seen = new Set<string>()
  for (const item of source) {
    const tag = String(item ?? '').trim().slice(0, TAG_MAX_LENGTH)
    if (!tag) continue
    const key = tagKey(tag)
    if (seen.has(key)) continue
    seen.add(key)
    tags.push(tag)
    if (tags.length >= TAG_MAX_COUNT) break
  }
  return tags
}

export function tagKey(tag: string): string {
  return String(tag || '').trim().toLowerCase()
}

export function tagsForItem(item: Pick<OpsAccountHealthItem, 'tags'> | null | undefined): string[] {
  return normalizeTags(item?.tags ?? [])
}

export function matchesTagFilter(tags: string[], filters: string[], mode: TagMatchMode): boolean {
  const normalizedFilters = normalizeTags(filters).map(tagKey)
  if (normalizedFilters.length === 0) return true
  const tagSet = new Set(normalizeTags(tags).map(tagKey))
  if (mode === 'all') {
    return normalizedFilters.every(tag => tagSet.has(tag))
  }
  return normalizedFilters.some(tag => tagSet.has(tag))
}

export function availableTagOptions<T extends Pick<OpsAccountHealthItem, 'tags'>>(items: T[]): Array<{ tag: string; count: number }> {
  const ordered = [...DEFAULT_TAG_OPTIONS]
  const seen = new Set(ordered.map(tagKey))
  for (const item of items) {
    for (const tag of tagsForItem(item)) {
      const key = tagKey(tag)
      if (seen.has(key)) continue
      seen.add(key)
      ordered.push(tag)
    }
  }
  return ordered.map(tag => ({
    tag,
    count: items.filter(item => tagsForItem(item).some(itemTag => tagKey(itemTag) === tagKey(tag))).length
  }))
}
