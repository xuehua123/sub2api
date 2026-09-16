import { describe, expect, it } from 'vitest'
import { groupMonitorCards } from '../monitorGroups'
import type { Provider, UserMonitorView } from '@/api/channelMonitor'

function monitor(id: number, provider: Provider): UserMonitorView {
  return { id, provider, name: 'Same name', group_name: '', primary_model: 'model', primary_status: 'operational', primary_latency_ms: 100, primary_ping_latency_ms: 50, availability_7d: 100, extra_models: [], timeline: [] }
}

describe('monitor platform grouping', () => {
  it('keeps fixed platform and ID order across shuffled refreshes and health changes', () => {
    const items = [monitor(9, 'gemini'), monitor(7, 'openai'), monitor(3, 'anthropic'), monitor(2, 'openai')]
    const original = [...items]
    const first = groupMonitorCards(items)
    expect(first.map(group => group.provider)).toEqual(['openai', 'anthropic', 'gemini'])
    expect(first.map(group => group.items.map(item => item.id))).toEqual([[2, 7], [3], [9]])
    expect(items).toEqual(original)
    const refreshed = [...items].reverse().map(item => ({ ...item, primary_status: 'error' as const, primary_latency_ms: 5000, availability_7d: 25 }))
    expect(groupMonitorCards(refreshed).map(group => group.items.map(item => item.id))).toEqual([[2, 7], [3], [9]])
  })

  it('keeps other and unknown platforms visible with deterministic ordering', () => {
    const items = [monitor(4, 'zeta' as Provider), monitor(3, 'grok'), monitor(2, 'alpha' as Provider), monitor(1, 'gemini')]
    expect(groupMonitorCards(items).map(group => group.provider)).toEqual(['gemini', 'grok', 'alpha', 'zeta'])
    expect(groupMonitorCards([])).toEqual([])
  })
})
