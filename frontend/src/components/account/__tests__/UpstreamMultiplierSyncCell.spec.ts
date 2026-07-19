import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UpstreamMultiplierSyncCell from '../UpstreamMultiplierSyncCell.vue'
import type { UpstreamAccountBinding } from '@/api/admin/upstreamConnections'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const binding = (overrides: Partial<UpstreamAccountBinding> = {}): UpstreamAccountBinding => ({
  id: 1,
  account_id: 11,
  connection_id: 3,
  remote_token_id: 'remote-key',
  remote_token_name: 'primary',
  resolution_kind: 'fixed',
  remote_group_id: 'vip',
  remote_group_name: 'VIP',
  fallback_groups: [],
  observed_multiplier: 0.01,
  confidence: 'exact',
  source: 'sub2api',
  apply_policy: 'auto',
  status: 'ready',
  sync_failures: 0,
  last_error: '',
  resolution_details: { rate_confidence: 'default' },
  observed_at: '2026-07-19T00:00:00Z',
  fresh_until: '2099-01-01T00:00:00Z',
  ...overrides
})

describe('UpstreamMultiplierSyncCell', () => {
  it('marks a fresh matching default multiplier as synchronized with default source', () => {
    const wrapper = mount(UpstreamMultiplierSyncCell, {
      props: {
        accountMultiplier: 0.01,
        connectionName: 'Primary upstream',
        binding: binding()
      },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.text()).toContain('0.01x')
    expect(wrapper.get('[data-testid="upstream-multiplier-sync-status"]').text()).toContain(
      'admin.accounts.upstreamMultiplierSync.synchronizedDefault'
    )
    expect(wrapper.get('[data-testid="upstream-multiplier-sync-status"]').classes()).toContain('text-emerald-600')
  })

  it('marks a fresh matching override multiplier as synchronized with override source', () => {
    const wrapper = mount(UpstreamMultiplierSyncCell, {
      props: {
        accountMultiplier: 0.18,
        binding: binding({
          observed_multiplier: 0.18,
          resolution_details: { rate_confidence: 'override' }
        })
      },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.get('[data-testid="upstream-multiplier-sync-status"]').text()).toContain(
      'admin.accounts.upstreamMultiplierSync.synchronizedOverride'
    )
  })

  it('flags a fresh auto-syncable upstream multiplier that differs from the account billing multiplier', () => {
    const wrapper = mount(UpstreamMultiplierSyncCell, {
      props: { accountMultiplier: 0.02, binding: binding() },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.get('[data-testid="upstream-multiplier-sync-status"]').text()).toContain(
      'admin.accounts.upstreamMultiplierSync.mismatch'
    )
    expect(wrapper.get('[data-testid="upstream-multiplier-sync-status"]').classes()).toContain('text-amber-600')
  })

  it('does not treat matching unavailable multipliers as synchronized', () => {
    const wrapper = mount(UpstreamMultiplierSyncCell, {
      props: {
        accountMultiplier: 1,
        binding: binding({
          observed_multiplier: 1,
          resolution_details: { rate_confidence: 'unavailable' }
        })
      },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.get('[data-testid="upstream-multiplier-sync-status"]').text()).toContain(
      'admin.accounts.upstreamMultiplierSync.displayOnly'
    )
    expect(wrapper.get('[data-testid="upstream-multiplier-sync-status"]').classes()).toContain('text-amber-600')
  })

  it('maps legacy reported confidence to syncable override display', () => {
    const wrapper = mount(UpstreamMultiplierSyncCell, {
      props: {
        accountMultiplier: 0.01,
        binding: binding({
          resolution_details: { rate_confidence: 'reported' }
        })
      },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.get('[data-testid="upstream-multiplier-sync-status"]').text()).toContain(
      'admin.accounts.upstreamMultiplierSync.synchronizedOverride'
    )
  })

  it('keeps missing, stale, and failed observations distinguishable', async () => {
    const wrapper = mount(UpstreamMultiplierSyncCell, {
      props: { accountMultiplier: 0.01 },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.text()).toContain('admin.accounts.upstreamMultiplierSync.unbound')

    await wrapper.setProps({ binding: binding({ fresh_until: '2020-01-01T00:00:00Z' }) })
    expect(wrapper.text()).toContain('admin.accounts.upstreamMultiplierSync.stale')

    await wrapper.setProps({ binding: binding({ status: 'error', last_error: 'expired token' }) })
    expect(wrapper.text()).toContain('admin.accounts.upstreamMultiplierSync.failed')
    expect(wrapper.get('[data-testid="upstream-multiplier-sync-status"]').classes()).toContain('text-red-600')
  })
})
