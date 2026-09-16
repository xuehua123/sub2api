import type { Provider, UserMonitorView } from '@/api/channelMonitor'
import { PROVIDERS } from '@/constants/channelMonitor'

export function groupMonitorCards(items: readonly UserMonitorView[]): Array<{ provider: Provider; items: UserMonitorView[] }> {
  const byProvider = new Map<Provider, UserMonitorView[]>()
  for (const item of items) {
    const group = byProvider.get(item.provider) ?? []
    group.push(item)
    byProvider.set(item.provider, group)
  }
  return [...byProvider.entries()]
    .sort(([left], [right]) => {
      const leftIndex = PROVIDERS.indexOf(left)
      const rightIndex = PROVIDERS.indexOf(right)
      const leftRank = leftIndex < 0 ? PROVIDERS.length : leftIndex
      const rightRank = rightIndex < 0 ? PROVIDERS.length : rightIndex
      return leftRank - rightRank || (left < right ? -1 : left > right ? 1 : 0)
    })
    .map(([provider, monitors]) => ({ provider, items: monitors.sort((left, right) => left.id - right.id) }))
}
