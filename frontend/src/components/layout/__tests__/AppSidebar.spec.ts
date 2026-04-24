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
  it('keeps user-facing channel monitor entries behind feature flags', () => {
    expect(componentSource).toContain("path: '/available-channels'")
    expect(componentSource).toContain("label: t('nav.availableChannels')")
    expect(componentSource).toContain('featureFlag: flagAvailableChannels')

    expect(componentSource).toContain("path: '/monitor'")
    expect(componentSource).toContain("label: t('nav.channelStatus')")
    expect(componentSource).toContain('featureFlag: flagChannelMonitor')
  })

  it('keeps admin channel monitor entry behind the channel monitor flag', () => {
    expect(componentSource).toContain("path: '/admin/channels/monitor'")
    expect(componentSource).toContain("label: t('nav.channelMonitor')")
    expect(componentSource).toContain('featureFlag: flagChannelMonitor')
  })

  it.each(['availableChannels', 'channelStatus', 'channelMonitor'])(
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
