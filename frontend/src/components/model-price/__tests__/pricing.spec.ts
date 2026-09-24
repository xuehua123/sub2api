import { describe, it, expect } from 'vitest'
import {
  kind,
  unit,
  estimate,
  comparePrices,
  issues,
  money,
  type Usage,
} from '../pricing'
import { model } from './fixtures'
const usage: Usage = {
  input: 1e6,
  output: 1e5,
  cache_read: 0,
  cache_write: 0,
  cache_write_5m: 0,
  cache_write_1h: 0,
  image_input: 0,
  image_output: 0,
  quantity: 1,
  requests: 1,
}
describe('price estimates', () => {
  it('multiplies token usage by request count without multiplying rates again', () => {
    expect(
      estimate(model(), 'base', { ...usage, requests: 3 }).total,
    ).toBeCloseTo(17.64)
  })
  it('distinguishes missing prices from zero', () => {
    const m = model()
    m.actual.output_cny_per_m = null
    expect(estimate(m, 'base', usage).total).toBeNull()
    m.actual.output_cny_per_m = 0
    expect(estimate(m, 'base', usage).total).toBe(4.2)
    expect(money(0)).not.toBe('待定价')
  })
  it('does not guess media billing from the model name', () => {
    const m = model({ name: 'video-audio-image', billing_mode: 'per_request' })
    expect(unit(m)).toBe('次')
    expect(kind(model({ billing_mode: 'unknown' }))).toBe('unknown')
  })
  it('calculates image and video quantities using explicit billing modes', () => {
    for (const mode of ['image', 'video', 'per_request']) {
      const m = model({ billing_mode: mode })
      m.actual.per_request_cny = 0.25
      expect(
        estimate(m, 'base', { ...usage, quantity: 4, requests: 2 }).total,
      ).toBe(2)
    }
  })
  it('rejects negative, invalid and fractional token inputs', () => {
    for (const value of [-1, NaN, Infinity, 1.5])
      expect(
        estimate(model(), 'base', { ...usage, input: value }).total,
      ).toBeNull()
  })
  it('does not use tier prices to fill missing base values', () => {
    const m = model()
    m.actual.input_cny_per_m = null
    m.price_tiers = [
      {
        key: 'fast',
        label: 'Fast',
        actual: model().actual,
        official: model().official,
      },
    ]
    expect(estimate(m, 'base', usage).total).toBeNull()
    expect(estimate(m, 'fast', usage).total).toBeCloseTo(5.88)
  })
  it('requires an explicit long-context tier beyond its threshold', () => {
    const m = model()
    m.price_tiers = [
      {
        key: 'long',
        label: 'Long',
        threshold_tokens: 200000,
        actual: { ...m.actual, input_cny_per_m: 8.4 },
        official: m.official,
      },
    ]
    expect(estimate(m, 'base', usage).total).toBeNull()
    expect(estimate(m, 'long', usage).total).toBeCloseTo(10.08)
    expect(estimate(m, 'long', { ...usage, input: 200000 }).total).toBeNull()
    expect(
      estimate(m, 'base', { ...usage, input: 200000 }).total,
    ).not.toBeNull()
  })
  it('includes cache usage once and preserves a zero cache rate', () => {
    const m = model()
    m.actual.cache_read_cny_per_m = 0
    expect(
      estimate(m, 'base', { ...usage, input: 0, output: 0, cache_read: 1e6 })
        .total,
    ).toBe(0)
  })
  it('keeps unknown prices last when sorting, explicit zero first', () => {
    const missing = model({ name: 'missing' })
    missing.actual.input_cny_per_m = null
    const zero = model({ name: 'zero' })
    zero.actual.input_cny_per_m = 0
    expect(
      [missing, model(), zero]
        .sort((a, b) => comparePrices(a, b, 'input'))
        .map((m) => m.name),
    ).toEqual(['zero', 'test-model', 'missing'])
  })
  it('does not exempt incomplete custom prices from warnings', () => {
    const m = model({ pricing_source: 'custom' })
    m.actual.output_cny_per_m = null
    expect(issues(m)).toContain('missing')
  })
})
