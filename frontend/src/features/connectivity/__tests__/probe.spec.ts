import { afterEach, describe, expect, it, vi } from 'vitest'
import { probeEndpoint } from '../probe'

describe('probeEndpoint', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('uses the credential-free fixed probe protocol and measures the full valid response', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ ok: true, client_ip: '8.8.8.8' }),
      { status: 200, headers: { 'Content-Type': 'application/json; charset=utf-8' } },
    ))
    const times = [100, 145]

    const result = await probeEndpoint('https://api.example.com/.well-known/sub2api/edge-probe', 5000, undefined, {
      fetchImpl,
      now: () => times.shift() ?? 145,
    })

    expect(result).toEqual({ kind: 'success', durationMs: 45, clientIP: '8.8.8.8', clientLocation: null })
    const [requestURL, init] = fetchImpl.mock.calls[0]
    const url = new URL(requestURL)
    expect(url.origin + url.pathname).toBe('https://api.example.com/.well-known/sub2api/edge-probe')
    expect(url.searchParams.get('nonce')).toMatch(/^[0-9a-f]{32}$/)
    expect(init).toMatchObject({
      method: 'GET',
      mode: 'cors',
      credentials: 'omit',
      cache: 'no-store',
      redirect: 'error',
      referrerPolicy: 'no-referrer',
    })
    expect(init.signal).toBeInstanceOf(AbortSignal)
  })

  it('classifies HTTP, rate-limit, network, and protocol failures without throwing', async () => {
    await expect(probeEndpoint('https://api.example.com/probe', 5000, undefined, {
      fetchImpl: vi.fn().mockResolvedValue(new Response('', { status: 429 })),
    })).resolves.toEqual({ kind: 'rate_limited' })

    await expect(probeEndpoint('https://api.example.com/probe', 5000, undefined, {
      fetchImpl: vi.fn().mockResolvedValue(new Response('', { status: 503 })),
    })).resolves.toEqual({ kind: 'http_error' })

    await expect(probeEndpoint('https://api.example.com/probe', 5000, undefined, {
      fetchImpl: vi.fn().mockRejectedValue(new TypeError('Failed to fetch')),
    })).resolves.toEqual({ kind: 'network_or_cors' })

    for (const response of [
      new Response('{"ok":true,"client_ip":null}', { status: 200, headers: { 'Content-Type': 'text/plain' } }),
      new Response('{"ok":false,"client_ip":null}', { status: 200, headers: { 'Content-Type': 'application/json' } }),
      new Response('{"ok":true,"client_ip":"not-an-ip"}', { status: 200, headers: { 'Content-Type': 'application/json' } }),
      new Response('{"ok":true,"client_ip":null,"client_location":"CN"}', { status: 200, headers: { 'Content-Type': 'application/json' } }),
      new Response('{"ok":true,"client_ip":null,"client_location":{"country_code":"CN","country":"' + '中'.repeat(70) + '"}}', { status: 200, headers: { 'Content-Type': 'application/json' } }),
      new Response(JSON.stringify({ ok: true, client_ip: null, client_location: { country_code: 'CN', country: String.fromCharCode(0) } }), { status: 200, headers: { 'Content-Type': 'application/json' } }),
      new Response(JSON.stringify({ ok: true, client_ip: null, client_location: { country_code: 'CN', country: `China\u2028hidden` } }), { status: 200, headers: { 'Content-Type': 'application/json' } }),
      new Response(JSON.stringify({ ok: true, client_ip: null, client_location: { country_code: 'CN', country: `China\u2029hidden` } }), { status: 200, headers: { 'Content-Type': 'application/json' } }),
      new Response(JSON.stringify({ ok: true, client_ip: null, client_location: { country_code: 'CN', country: `\u2028China` } }), { status: 200, headers: { 'Content-Type': 'application/json' } }),
      new Response('x'.repeat(1025), { status: 200, headers: { 'Content-Type': 'application/json' } }),
      new Response(new Uint8Array([0xff]), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    ]) {
      await expect(probeEndpoint('https://api.example.com/probe', 5000, undefined, {
        fetchImpl: vi.fn().mockResolvedValue(response),
      })).resolves.toEqual({ kind: 'protocol_error' })
    }
  })

  it('keeps an oversized response classified as a protocol error when stream cancellation fails', async () => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new Uint8Array(1025))
      },
      cancel() {
        throw new Error('stream cancellation failed')
      },
    })

    await expect(probeEndpoint('https://api.example.com/probe', 5000, undefined, {
      fetchImpl: vi.fn().mockResolvedValue(new Response(body, {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })),
    })).resolves.toEqual({ kind: 'protocol_error' })
  })

  it('distinguishes timeout from caller cancellation', async () => {
    vi.useFakeTimers()
    const fetchImpl = vi.fn((_url: string, init?: RequestInit) => new Promise<Response>((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
    }))

    const timeoutResult = probeEndpoint('https://api.example.com/probe', 1000, undefined, { fetchImpl })
    await vi.advanceTimersByTimeAsync(1000)
    await expect(timeoutResult).resolves.toEqual({ kind: 'timeout' })

    const controller = new AbortController()
    const cancelledResult = probeEndpoint('https://api.example.com/probe', 1000, controller.signal, { fetchImpl })
    controller.abort()
    await expect(cancelledResult).resolves.toEqual({ kind: 'cancelled' })
  })

  it('carries a validated client location on success', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(new Response(
      JSON.stringify({
        ok: true,
        client_ip: '8.8.8.8',
        client_location: { country_code: 'CN', country: '中国', region: '广东', city: '深圳' },
      }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    ))

    const result = await probeEndpoint('https://api.example.com/probe', 5000, undefined, {
      fetchImpl,
      now: () => 100,
    })
    expect(result).toEqual({
      kind: 'success',
      durationMs: 0,
      clientIP: '8.8.8.8',
      clientLocation: { country_code: 'CN', country: '中国', region: '广东', city: '深圳' },
    })
  })

  it('validates location limits by UTF-8 bytes rather than JavaScript characters', async () => {
    for (const country of ['a'.repeat(96), '中'.repeat(32)]) {
      const fetchImpl = vi.fn().mockResolvedValue(new Response(
        JSON.stringify({
          ok: true,
          client_ip: '8.8.8.8',
          client_location: { country_code: 'CN', country, region: '', city: '' },
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ))

      await expect(probeEndpoint('https://api.example.com/probe', 5000, undefined, {
        fetchImpl,
      })).resolves.toMatchObject({
        kind: 'success',
        clientLocation: { country },
      })
    }

    for (const country of ['a'.repeat(97), '中'.repeat(33)]) {
      const fetchImpl = vi.fn().mockResolvedValue(new Response(
        JSON.stringify({
          ok: true,
          client_ip: '8.8.8.8',
          client_location: { country_code: 'CN', country, region: '', city: '' },
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ))

      await expect(probeEndpoint('https://api.example.com/probe', 5000, undefined, {
        fetchImpl,
      })).resolves.toEqual({ kind: 'protocol_error' })
    }
  })

  it('rejects a probe body larger than the 1 KiB cap even when it is valid JSON', async () => {
    const oversized = JSON.stringify({
      ok: true,
      client_ip: null,
      client_location: { country_code: 'CN', country: '中'.repeat(1200) },
    })
    expect(oversized.length).toBeGreaterThan(1024)
    const fetchImpl = vi.fn().mockResolvedValue(new Response(oversized, {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }))

    await expect(probeEndpoint('https://api.example.com/probe', 5000, undefined, {
      fetchImpl,
    })).resolves.toEqual({ kind: 'protocol_error' })
  })
})
