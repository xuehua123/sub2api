import { shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import MonitorCardGrid from '../MonitorCardGrid.vue'
import type { Provider, UserMonitorView } from '@/api/channelMonitor'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))

function monitor(id: number, provider: Provider): UserMonitorView {
  return { id, provider, name: 'Monitor', group_name: '', primary_model: 'model', primary_status: 'operational', primary_latency_ms: 10, primary_ping_latency_ms: 5, availability_7d: 99, extra_models: [], timeline: [] }
}

describe('MonitorCardGrid platform sections', () => {
  it('renders named sections, preserves order after refresh and forwards the selected monitor', async () => {
    const items = [monitor(9, 'gemini'), monitor(7, 'openai'), monitor(3, 'anthropic'), monitor(2, 'openai')]
    const wrapper = shallowMount(MonitorCardGrid, {
      props: { items, window: '7d', countdownSeconds: 30, loading: false, detailCache: {} },
      global: { stubs: { MonitorCard: { props: ['item', 'availabilityValue'], emits: ['click'], template: '<button :data-monitor-id="item.id" @click="$emit(\'click\')">{{ availabilityValue }}</button>' } } },
    })
    expect(wrapper.findAll('h2').map(heading => heading.text())).toEqual(['Codex / OpenAI2', 'CC / Claude1', 'Gemini1'])
    const order = () => wrapper.findAll('[data-monitor-id]').map(card => card.attributes('data-monitor-id'))
    expect(order()).toEqual(['2', '7', '3', '9'])
    await wrapper.get('[data-monitor-id="7"]').trigger('click')
    expect(wrapper.emitted('cardClick')?.[0]).toEqual([items[1]])
    await wrapper.setProps({ items: [...items].reverse().map(item => ({ ...item, primary_status: 'error', availability_7d: 20 })), loading: true })
    expect(order()).toEqual(['2', '7', '3', '9'])
    expect(wrapper.findAll('section')).toHaveLength(3)
    await wrapper.setProps({ window: '15d', detailCache: { 7: { id: 7, name: 'Monitor', provider: 'openai', group_name: '', models: [{ model: 'model', latest_status: 'operational', latest_latency_ms: 10, availability_7d: 99, availability_15d: 88, availability_30d: 77, avg_latency_7d_ms: 10 }] } } })
    expect(wrapper.get('[data-monitor-id="7"]').text()).toBe('88')
    await wrapper.setProps({ items: [], loading: false })
    expect(wrapper.find('empty-state-stub').exists()).toBe(true)
    expect(wrapper.findAll('section')).toHaveLength(0)
  })
})
