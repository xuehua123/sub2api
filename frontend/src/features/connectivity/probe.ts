import type { ProbeAttempt } from './types'

const maxProbeResponseBytes = 128

interface ProbeEndpointDependencies {
  fetchImpl?: typeof fetch
  now?: () => number
}

export async function probeEndpoint(
  probeURL: string,
  timeoutMs: number,
  parentSignal?: AbortSignal,
  dependencies: ProbeEndpointDependencies = {},
): Promise<ProbeAttempt> {
  if (parentSignal?.aborted) return { kind: 'cancelled' }

  const fetchImpl = dependencies.fetchImpl ?? fetch
  const now = dependencies.now ?? (() => performance.now())
  const controller = new AbortController()
  let timedOut = false
  const handleParentAbort = () => controller.abort()
  parentSignal?.addEventListener('abort', handleParentAbort, { once: true })
  const timeout = setTimeout(() => {
    timedOut = true
    controller.abort()
  }, timeoutMs)

  const start = now()
  try {
    const url = new URL(probeURL)
    url.search = ''
    url.hash = ''
    url.searchParams.set('nonce', randomNonce())
    const response = await fetchImpl(url.toString(), {
      method: 'GET',
      mode: 'cors',
      credentials: 'omit',
      cache: 'no-store',
      redirect: 'error',
      referrerPolicy: 'no-referrer',
      signal: controller.signal,
    })
    if (parentSignal?.aborted) return { kind: 'cancelled' }
    if (response.status === 429) return { kind: 'rate_limited' }
    if (response.status !== 200) return { kind: 'http_error' }
    if (!isJSONContentType(response.headers.get('Content-Type'))) {
      return { kind: 'protocol_error' }
    }

    const body = await readLimitedBody(response, maxProbeResponseBytes)
    if (body === null) return { kind: 'protocol_error' }
    if (parentSignal?.aborted) return { kind: 'cancelled' }
    const payload = parseProbePayload(body)
    if (!payload.valid) return { kind: 'protocol_error' }

    return {
      kind: 'success',
      durationMs: Math.max(0, now() - start),
      clientIP: payload.clientIP,
    }
  } catch {
    if (timedOut) return { kind: 'timeout' }
    if (parentSignal?.aborted) return { kind: 'cancelled' }
    return { kind: 'network_or_cors' }
  } finally {
    clearTimeout(timeout)
    parentSignal?.removeEventListener('abort', handleParentAbort)
  }
}

function randomNonce(): string {
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('')
}

function isJSONContentType(value: string | null): boolean {
  return value?.split(';', 1)[0].trim().toLowerCase() === 'application/json'
}

async function readLimitedBody(response: Response, limit: number): Promise<string | null> {
  if (!response.body) {
    const buffer = new Uint8Array(await response.arrayBuffer())
    return buffer.byteLength <= limit ? decodeProbeBody(buffer) : null
  }

  const reader = response.body.getReader()
  const chunks: Uint8Array[] = []
  let total = 0
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      total += value.byteLength
      if (total > limit) {
        try {
          await reader.cancel()
        } catch {
          // The response is already invalid; cancellation is only best-effort cleanup.
        }
        return null
      }
      chunks.push(value)
    }
  } finally {
    reader.releaseLock()
  }

  const combined = new Uint8Array(total)
  let offset = 0
  for (const chunk of chunks) {
    combined.set(chunk, offset)
    offset += chunk.byteLength
  }
  return decodeProbeBody(combined)
}

function decodeProbeBody(body: Uint8Array): string | null {
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(body)
  } catch {
    return null
  }
}

function parseProbePayload(body: string): { valid: true; clientIP: string | null } | { valid: false } {
  let value: unknown
  try {
    value = JSON.parse(body)
  } catch {
    return { valid: false }
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) return { valid: false }
  const payload = value as Record<string, unknown>
  if (payload.ok !== true || !Object.prototype.hasOwnProperty.call(payload, 'client_ip')) {
    return { valid: false }
  }
  if (payload.client_ip === null) return { valid: true, clientIP: null }
  if (typeof payload.client_ip !== 'string') return { valid: false }
  const normalized = normalizeIPAddress(payload.client_ip)
  return normalized ? { valid: true, clientIP: normalized } : { valid: false }
}

function normalizeIPAddress(value: string): string | null {
  if (value !== value.trim() || value === '') return null
  if (/^(?:0|[1-9]\d{0,2})(?:\.(?:0|[1-9]\d{0,2})){3}$/.test(value)) {
    const parts = value.split('.').map(Number)
    return parts.every((part) => part <= 255) ? parts.join('.') : null
  }
  if (!value.includes(':') || /[\s%\[\]\/]/.test(value)) return null
  try {
    const parsed = new URL(`http://[${value}]/`)
    const hostname = parsed.hostname.replace(/^\[|\]$/g, '').toLowerCase()
    return hostname.includes(':') ? hostname : null
  } catch {
    return null
  }
}
