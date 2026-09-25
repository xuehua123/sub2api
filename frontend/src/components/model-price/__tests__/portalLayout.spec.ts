import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import Layout from '../PricePortalLayout.vue'
vi.mock('@/stores', () => ({
  useAppStore: () => ({
    siteName: '皮皮虾 AI',
    siteLogo: '/brand/ppx-logo.webp',
    docUrl: 'https://doc.psydo.top/',
  }),
  useAuthStore: () => ({ isAuthenticated: true, isAdmin: false }),
}))
function view(publicPage = true) {
  return mount(Layout, {
    props: { publicPage },
    slots: { default: '<div data-test="page-content">Page content</div>' },
    global: {
      stubs: {
        AppLayout: { template: '<div data-test="console-layout"><aside data-test="sidebar"/><header data-test="mobile-menu"/><slot/></div>' },
        Icon: true, RouterLink: { template: '<a><slot/></a>' }
      },
    },
  })
}
beforeEach(() => {
  localStorage.clear()
  document.documentElement.classList.remove('dark', 'ppx-price-page')
})
describe('PPX pricing layout', () => {
  it('keeps console navigation for account pages without changing the console theme', () => {
    localStorage.setItem('ppx-theme', 'dark')
    const w = view(false)
    expect(w.find('[data-test="console-layout"]').exists()).toBe(true)
    expect(w.find('[data-test="sidebar"]').exists()).toBe(true)
    expect(w.find('[data-test="mobile-menu"]').exists()).toBe(true)
    expect(w.find('[data-test="page-content"]').exists()).toBe(true)
    expect(w.find('.ppx-price-header').exists()).toBe(false)
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(document.documentElement.classList.contains('ppx-price-page')).toBe(false)
    document.documentElement.classList.add('dark')
    w.unmount()
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })
  it('keeps public pages independent of the console navigation', () => {
    const w = view(true)
    expect(w.find('[data-test="console-layout"]').exists()).toBe(false)
    expect(w.find('.ppx-price-header').exists()).toBe(true)
    w.unmount()
  })
  it('uses the homepage theme preference and restores console appearance after leaving', () => {
    localStorage.setItem('ppx-theme', 'dark')
    const w = view()
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(w.text()).toContain('皮皮虾 AI')
    w.unmount()
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(document.documentElement.classList.contains('ppx-price-page')).toBe(
      false,
    )
  })
  it('persists theme changes under the same key as home and docs', async () => {
    const w = view()
    await w.get('[aria-label="切换浅色主题"]').trigger('click')
    expect(localStorage.getItem('ppx-theme')).toBe('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(w.get('.ppx-price-portal').attributes('data-theme')).toBe('light')
    w.unmount()
  })
  it('honors a saved light preference rather than forcing dark', () => {
    localStorage.setItem('ppx-theme', 'light')
    const w = view()
    expect(w.get('.ppx-price-portal').attributes('data-theme')).toBe('light')
    w.unmount()
  })
})
