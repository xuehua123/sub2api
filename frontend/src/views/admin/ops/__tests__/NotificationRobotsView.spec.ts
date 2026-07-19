import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import NotificationRobotsView from '../NotificationRobotsView.vue'

const {
  getConnectionBalance,
  getAccountHealth,
  probeConnectionBalance,
  updateConnectionAlert
} = vi.hoisted(() => ({
  getConnectionBalance: vi.fn(),
  getAccountHealth: vi.fn(),
  probeConnectionBalance: vi.fn(),
  updateConnectionAlert: vi.fn()
}))

const balanceSettings = {
  enabled: true,
  default_threshold_usd: 10,
  rate_limit_per_hour: 12,
  notification: {
    enterprise_wechat_enabled: false,
    enterprise_wechat_webhook_url: '',
    mention_all_on_low_balance: false
  },
}

const healthSettings = {
  enabled: true,
  mode: 'smart',
  burst: { enabled: true, window_minutes: 1, min_requests: 10, error_rate_percent: 50, upstream_error_rate_percent: 25, cooldown_minutes: 1, bypass_digest: true },
  degrade: { enabled: true, window_minutes: 10, min_requests: 20, success_rate_min_percent: 90, error_rate_percent: 20, upstream_error_rate_percent: 10, cooldown_minutes: 10 },
  recovery: { enabled: true, window_minutes: 30, min_requests: 10, success_rate_min_percent: 98, notify_opened_accounts: true, notify_closed_accounts: true, cooldown_minutes: 30 },
  probe: { enabled: true, interval_minutes: 30, max_per_run: 2, timeout_seconds: 20, model_id: 'gpt-5.4-mini', mode: 'default', prompt: '' },
  notification: { enterprise_wechat_enabled: false, enterprise_wechat_webhook_url: '', mention_all_on_immediate: false },
  rate_limit_per_hour: 12
}

const connectionItem = {
  connection_id: 7,
  name: 'Shared NewAPI wallet',
  provider: 'newapi',
  status: 'ready',
  last_error: '',
  sync_enabled: true,
  sync_interval_seconds: 300,
  binding_count: 2,
  bound_account_ids: [11, 12],
  wallet_amount: 6.25,
  wallet_currency: 'USD',
  wallet_usd: 6.25,
  wallet_unlimited: false,
  wallet_source: 'newapi_user_self',
  wallet_reliability: 'strong',
  wallet_observed_at: '2026-07-19T00:00:00Z',
  alert: {
    enabled: true,
    threshold_usd: 10,
    uses_default_threshold: true,
    eligible: true,
    snapshot_fresh: true,
    low: true
  }
}

vi.mock('@/api/admin/ops', () => ({
  getUpstreamConnectionBalanceMonitor: getConnectionBalance,
  getAccountHealth,
  probeUpstreamConnectionBalance: probeConnectionBalance,
  updateUpstreamConnectionBalanceAlert: updateConnectionAlert,
  updateUpstreamConnectionBalanceSettings: vi.fn(),
  updateAccountHealthSettings: vi.fn(),
  testUpstreamConnectionBalanceEnterpriseWeChat: vi.fn(),
  testAccountHealthEnterpriseWeChat: vi.fn(),
  opsAPI: {},
  default: {}
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() })
}))

function mountView() {
  return mount(NotificationRobotsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: true
      }
    }
  })
}

describe('NotificationRobotsView', () => {
  beforeEach(() => {
    getConnectionBalance.mockReset().mockResolvedValue({
      generated_at: '2026-07-19T00:00:00Z',
      settings: balanceSettings,
      items: [{ ...connectionItem, alert: { ...connectionItem.alert } }],
      total: 1,
      page: 1,
      page_size: 20,
      summary: {
        total_connections: 1,
        monitored_connections: 1,
        low_balance_connections: 1,
        failed_connections: 0,
        stale_connections: 0,
        unlimited_connections: 0
      }
    })
    getAccountHealth.mockReset().mockResolvedValue({ settings: healthSettings, items: [] })
    probeConnectionBalance.mockReset().mockResolvedValue({ ...connectionItem, alert: { ...connectionItem.alert } })
    updateConnectionAlert.mockReset().mockResolvedValue({ ...connectionItem.alert })
  })

  it('renders shared connection wallets without legacy account balance controls', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('上游余额规则'))!.trigger('click')

    expect(getConnectionBalance).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('Shared NewAPI wallet')
    expect(wrapper.text()).toContain('$6.25')
    expect(wrapper.text()).toContain('绑定 2 个账号')
    expect(wrapper.text()).not.toContain('查询方式')
    expect(wrapper.text()).not.toContain('账号余额控制台')
    expect(wrapper.text()).not.toContain('仅探测当前可调度账号')
  })

  it('refreshes the shared connection rather than probing an account', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('上游余额规则'))!.trigger('click')

    await wrapper.get('button[aria-label="刷新连接余额"]').trigger('click')
    await flushPromises()

    expect(probeConnectionBalance).toHaveBeenCalledWith(7)
    expect(getConnectionBalance).toHaveBeenCalledTimes(2)
  })

  it('refreshes filtered rows without overwriting unsaved global settings', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('上游余额规则'))!.trigger('click')

    const initialResponse = await getConnectionBalance.mock.results[0]!.value
    getConnectionBalance.mockResolvedValueOnce({
      ...initialResponse,
      settings: { ...balanceSettings, default_threshold_usd: 1 }
    })
    await wrapper.get('[data-testid="balance-default-threshold"]').setValue('33')
    await wrapper.get('[data-testid="balance-only-low"]').setValue(true)
    await flushPromises()

    expect((wrapper.get('[data-testid="balance-default-threshold"]').element as HTMLInputElement).value).toBe('33')
  })

  it('discards unsaved health and balance settings from the authoritative reload', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="health-burst-window"]').setValue('5')
    await wrapper.findAll('nav button')[1]!.trigger('click')
    await wrapper.get('[data-testid="balance-default-threshold"]').setValue('33')

    const initialBalance = await getConnectionBalance.mock.results[0]!.value
    getConnectionBalance.mockResolvedValueOnce({
      ...initialBalance,
      settings: { ...balanceSettings, default_threshold_usd: 22 }
    })
    getAccountHealth.mockResolvedValueOnce({
      settings: {
        ...healthSettings,
        burst: { ...healthSettings.burst, window_minutes: 7 }
      },
      items: []
    })

    await wrapper.get('[data-testid="discard-settings"]').trigger('click')
    await flushPromises()

    expect((wrapper.get('[data-testid="balance-default-threshold"]').element as HTMLInputElement).value).toBe('22')
    await wrapper.findAll('nav button')[0]!.trigger('click')
    expect((wrapper.get('[data-testid="health-burst-window"]').element as HTMLInputElement).value).toBe('7')
    expect(wrapper.find('[data-testid="discard-settings"]').exists()).toBe(false)
  })

  it('keeps the full settings load when a newer balance-list request finishes first', async () => {
    let resolveHealth!: (value: { settings: typeof healthSettings; items: never[] }) => void
    getAccountHealth.mockReturnValueOnce(new Promise(resolve => { resolveHealth = resolve }))

    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('上游余额规则'))!.trigger('click')
    await wrapper.get('[data-testid="balance-only-low"]').setValue(true)
    await flushPromises()

    expect(wrapper.findAll('button').find(button => button.text().includes('保存配置'))!.attributes('disabled')).toBeDefined()

    resolveHealth({
      settings: {
        ...healthSettings,
        burst: { ...healthSettings.burst, window_minutes: 7 }
      },
      items: []
    })
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('账号健康规则'))!.trigger('click')

    expect((wrapper.get('[data-testid="health-burst-window"]').element as HTMLInputElement).value).toBe('7')
    await wrapper.get('[data-testid="health-burst-window"]').setValue('8')
    expect(wrapper.findAll('button').find(button => button.text().includes('保存配置'))!.attributes('disabled')).toBeUndefined()
  })

  it('exposes independent health rule switches and thresholds in one place', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="health-burst-enabled"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="health-degrade-enabled"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="health-recovery-enabled"]').exists()).toBe(true)
    await wrapper.get('[data-testid="health-burst-window"]').setValue('5')
    expect(wrapper.text()).toContain('错误率达到 %')
    expect(wrapper.text()).toContain('成功率低于 %')
    expect(wrapper.text()).toContain('5 分钟内至少 10 次请求')
  })
})
