import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UpstreamConnectionBalanceCell from '../UpstreamConnectionBalanceCell.vue'
import type { UpstreamConnection } from '@/api/admin/upstreamConnections'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const connection = (overrides: Partial<UpstreamConnection> = {}): UpstreamConnection => ({
  id: 7,
  name: 'Primary upstream',
  provider: 'sub2api',
  auth_mode: 'access_token',
  management_base_url: 'https://console.example.com',
  forwarding_base_url: 'https://api.example.com',
  credential_configured: true,
  credential_hint: 'token...',
  remote_user_id: '11',
  proxy_id: null,
  capabilities: {},
  status: 'ready',
  last_error: '',
  sync_enabled: true,
  sync_interval_seconds: 300,
  sync_failures: 0,
  version: 1,
  wallet_amount: 98.42,
  wallet_currency: 'USD',
  wallet_usd: 98.42,
  wallet_unlimited: false,
  wallet_source: 'sub2api:user_profile',
  wallet_reliability: 'exact',
  wallet_observed_at: '2026-07-19T00:00:00Z',
  last_discovered_at: '2026-07-19T00:00:00Z',
  last_synced_at: '2026-07-19T00:00:00Z',
  next_sync_at: '2026-07-19T00:05:00Z',
  created_at: '2026-07-19T00:00:00Z',
  updated_at: '2026-07-19T00:00:00Z',
  group_count: 1,
  binding_count: 1,
  bound_account_ids: [],
  groups: [],
  bindings: [],
  ...overrides
})

describe('UpstreamConnectionBalanceCell', () => {
  it('shows the shared wallet and exposes an icon-only refresh command', async () => {
    const wrapper = mount(UpstreamConnectionBalanceCell, {
      props: { connection: connection() },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.get('[data-testid="upstream-connection-balance"]').text()).toBe('$98.42')
    const refresh = wrapper.get('[data-testid="upstream-connection-balance-refresh"]')
    expect(refresh.attributes('aria-label')).toBe('admin.accounts.upstreamConnectionBalance.refresh')
    await refresh.trigger('click')
    expect(wrapper.emitted('refresh')).toHaveLength(1)
  })

  it('renders non-USD, unlimited, loading, and unbound states without probe controls', async () => {
    const wrapper = mount(UpstreamConnectionBalanceCell, {
      props: { connection: connection({ wallet_amount: 12.5, wallet_currency: 'CNY', wallet_usd: null }) },
      global: { stubs: { Icon: true } }
    })
    expect(wrapper.get('[data-testid="upstream-connection-balance"]').text()).toBe('¥12.50')

    await wrapper.setProps({ connection: connection({ wallet_amount: null, wallet_usd: null, wallet_unlimited: true }) })
    expect(wrapper.get('[data-testid="upstream-connection-balance"]').text()).toBe(
      'admin.accounts.upstreamConnectionBalance.unlimited'
    )

    await wrapper.setProps({ connection: null, loading: true })
    expect(wrapper.text()).toContain('admin.accounts.upstreamConnectionBalance.loading')
    expect(wrapper.find('[data-testid="upstream-connection-balance-refresh"]').exists()).toBe(false)

    await wrapper.setProps({ loading: false })
    expect(wrapper.text()).toContain('admin.accounts.upstreamConnectionBalance.unbound')

    await wrapper.setProps({ loadFailed: true })
    expect(wrapper.text()).toContain('admin.accounts.upstreamConnectionBalance.loadFailed')
  })

  it('marks a retained wallet as failed when the latest upstream probe has failed', () => {
    const wrapper = mount(UpstreamConnectionBalanceCell, {
      props: { connection: connection({ status: 'auth_error', last_error: 'expired token' }) },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.get('[data-testid="upstream-connection-balance"]').classes()).toContain('text-red-600')
  })

  it('marks a retained wallet as failed when the connection reports a sync error', () => {
    const wrapper = mount(UpstreamConnectionBalanceCell, {
      props: { connection: connection({ status: 'degraded', last_error: 'wallet sync failed' }) },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.get('[data-testid="upstream-connection-balance"]').classes()).toContain('text-red-600')
  })

  it('highlights a shared wallet below 50 USD', () => {
    const wrapper = mount(UpstreamConnectionBalanceCell, {
      props: { connection: connection({ wallet_amount: 49.99, wallet_usd: 49.99 }) },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.get('[data-testid="upstream-connection-balance"]').classes()).toContain('text-amber-600')
  })
})
