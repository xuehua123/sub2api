import { describe, expect, it } from 'vitest'
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import en from '../locales/en'
import zh from '../locales/zh'

const srcRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../../')
const sourceExtensions = new Set(['.vue', '.ts'])
const channelMonitorKeyPattern = /admin\.channelMonitor[\w.]+/g

describe('channel monitor locale keys', () => {
  const keys = collectChannelMonitorKeys(srcRoot)

  it('finds channel monitor keys in source files', () => {
    expect(keys.length).toBeGreaterThan(0)
  })

  it.each(keys)('has zh text for %s', key => {
    expect(resolveLocaleKey(zh, key)).toEqual(expect.any(String))
    expect(resolveLocaleKey(zh, key)).not.toBe(key)
  })

  it.each(keys)('has en text for %s', key => {
    expect(resolveLocaleKey(en, key)).toEqual(expect.any(String))
    expect(resolveLocaleKey(en, key)).not.toBe(key)
  })
})

function collectChannelMonitorKeys(root: string): string[] {
  const keys = new Set<string>()
  for (const file of walkFiles(root)) {
    if (!sourceExtensions.has(file.slice(file.lastIndexOf('.')))) {
      continue
    }
    const text = readFileSync(file, 'utf8')
    for (const match of text.matchAll(channelMonitorKeyPattern)) {
      keys.add(match[0])
    }
  }
  return [...keys].sort()
}

function walkFiles(root: string): string[] {
  if (!existsSync(root)) {
    return []
  }
  const out: string[] = []
  for (const entry of readdirSync(root)) {
    if (entry === 'node_modules' || entry === 'dist') {
      continue
    }
    const fullPath = join(root, entry)
    const stat = statSync(fullPath)
    if (stat.isDirectory()) {
      out.push(...walkFiles(fullPath))
      continue
    }
    out.push(fullPath)
  }
  return out
}

function resolveLocaleKey(messages: unknown, key: string): unknown {
  return key.split('.').reduce<unknown>((current, part) => {
    if (!current || typeof current !== 'object') {
      return undefined
    }
    return (current as Record<string, unknown>)[part]
  }, messages)
}
