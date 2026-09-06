import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import en from '../../../i18n/locales/en'
import zh from '../../../i18n/locales/zh'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon {')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar scroll position persistence', () => {
  it('binds a template ref to the sidebar nav element', () => {
    expect(componentSource).toContain('ref="sidebarNavRef"')
    expect(componentSource).toContain('sidebar-nav')
  })

  it('declares sidebarNavRef in script setup', () => {
    expect(componentSource).toContain("const sidebarNavRef = ref<HTMLElement | null>(null)")
  })

  it('saves scroll position on beforeUnmount', () => {
    expect(componentSource).toContain('onBeforeUnmount')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('sidebarNavRef.value.scrollTop')
  })

  it('restores scroll position on mount', () => {
    expect(componentSource).toContain('onMounted')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('nextTick')
  })
})

describe('AppSidebar collapsible groups', () => {
  it('lets the user collapse a group even while a child route is active', () => {
    // The expand state must come from the user's override first, falling back
    // to the active-route heuristic only when the user has not clicked yet.
    expect(componentSource).toContain('const groupExpandOverrides = ref<Map<string, boolean>>(new Map())')
    expect(componentSource).not.toContain('expandedGroups.value.has(item.path) || isGroupActive(item)')
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the version badge dropdown', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })
})

describe('AppSidebar channel monitor navigation', () => {
  it('uses feature-flagged shared self navigation for user and admin personal menus', () => {
    expect(componentSource).toContain('function buildSelfNavItems(withDashboard: boolean): NavItem[]')
    expect(componentSource).toContain('finalizeNav(buildSelfNavItems(true))')
    expect(componentSource).toContain('finalizeNav(buildSelfNavItems(false))')
    expect(componentSource).toContain("authStore.user?.payment_disabled !== true")
    expect(componentSource).toContain('makeSidebarFlag(FeatureFlags.payment)()')
    expect(componentSource).toContain('featureFlag: flagPayment')
  })

  it('keeps admin channel and order groups as expand-only sidebar groups', () => {
    expect(componentSource).toContain('expandOnly?: boolean')
    expect(componentSource).toContain('@click="handleGroupClick(item)"')
    expect(componentSource).toContain('function handleGroupClick(item: NavItem)')
    expect(componentSource).toContain("label: t('nav.channelManagement')")
    expect(componentSource).toContain('expandOnly: true')
    expect(componentSource).toContain("label: t('nav.orderManagement')")
    expect(componentSource).toContain('featureFlag: flagAdminPayment')
  })

  it('keeps user-facing channel monitor entries behind feature flags', () => {
    expect(componentSource).toContain("path: '/available-channels'")
    expect(componentSource).toContain("label: t('nav.availableChannels')")
    expect(componentSource).toContain('featureFlag: flagAvailableChannels')

    expect(componentSource).toContain("path: '/monitor'")
    expect(componentSource).toContain("label: t('nav.channelStatus')")
    expect(componentSource).toContain('featureFlag: flagChannelMonitor')
  })

  it('includes the documentation center in user and admin personal navigation', () => {
    expect(componentSource).toContain("path: '/docs'")
    expect(componentSource).toContain("label: t('nav.docsCenter')")
    expect(componentSource).toContain('resolvePublicDocumentationUrl')
    expect(componentSource).toContain('const personalNavItems = computed((): NavItem[] => finalizeNav(buildSelfNavItems(false)))')
  })

  it('includes the documentation center in the admin navigation', () => {
    expect(componentSource).toContain("path: '/docs'")
    expect(componentSource).toContain("label: t('nav.docsCenter')")
    expect(componentSource).toContain("const adminUnresolvedStatuses = ['open', 'needs_info', 'in_progress'] as const")
  })

  it('keeps admin channel monitor configuration entry visible even when user-facing monitor is disabled', () => {
    const adminChannelMonitorItem = componentSource.match(
      /\{\s*path: '\/admin\/channels\/monitor',[\s\S]*?label: t\('nav\.channelMonitor'\),[\s\S]*?icon: SignalIcon,[\s\S]*?\}/,
    )

    expect(adminChannelMonitorItem).not.toBeNull()
    expect(adminChannelMonitorItem?.[0]).not.toContain('featureFlag: flagChannelMonitor')
  })

  it.each(['availableChannels', 'channelStatus', 'channelMonitor', 'channelManagement', 'docsCenter', 'issueManagement'])(
    'has zh and en labels for nav.%s',
    key => {
      expect(resolveLocaleKey(zh, `nav.${key}`)).toEqual(expect.any(String))
      expect(resolveLocaleKey(en, `nav.${key}`)).toEqual(expect.any(String))
    },
  )
})

function resolveLocaleKey(messages: unknown, key: string): unknown {
  return key.split('.').reduce<unknown>((current, part) => {
    if (!current || typeof current !== 'object') {
      return undefined
    }
    return (current as Record<string, unknown>)[part]
  }, messages)
}
