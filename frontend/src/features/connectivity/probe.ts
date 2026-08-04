import type { ConnectivityClientLocation, ProbeAttempt } from './types'

// The probe response may now carry an optional client_location object. The
// hard cap is 1 KiB; anything larger is treated as a protocol error.
const maxProbeResponseBytes = 1024

const maxLocationFieldBytes = 96
const maxCountryCodeBytes = 8
const utf8Encoder = new TextEncoder()

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
      clientLocation: payload.clientLocation,
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

interface ProbePayload {
  valid: true
  clientIP: string | null
  clientLocation: ConnectivityClientLocation | null
}

function parseProbePayload(body: string): ProbePayload | { valid: false } {
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

  let clientIP: string | null = null
  if (payload.client_ip !== null) {
    if (typeof payload.client_ip !== 'string') return { valid: false }
    const normalized = normalizeIPAddress(payload.client_ip)
    if (!normalized) return { valid: false }
    clientIP = normalized
  }

  let clientLocation: ConnectivityClientLocation | null = null
  if (Object.prototype.hasOwnProperty.call(payload, 'client_location')) {
    if (payload.client_location !== null) {
      const parsedLocation = parseClientLocation(payload.client_location)
      if (parsedLocation === null) return { valid: false }
      clientLocation = parsedLocation
    }
  }

  return { valid: true, clientIP, clientLocation }
}

// parseClientLocation returns null when the value is present but malformed:
// wrong type, over-length fields, or abnormal Unicode control characters. An
// absent/empty location is represented by null (region unknown), never by a
// malformed object.
function parseClientLocation(value: unknown): ConnectivityClientLocation | null {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return null
  const record = value as Record<string, unknown>
  const countryCode = locationString(record.country_code, maxCountryCodeBytes)
  const country = locationString(record.country, maxLocationFieldBytes)
  const region = locationString(record.region, maxLocationFieldBytes)
  const city = locationString(record.city, maxLocationFieldBytes)
  if (countryCode === null || country === null || region === null || city === null) return null
  return { country_code: countryCode, country, region, city }
}

// locationString returns '' for missing/null fields, the trimmed value for a
// valid bounded string, and null when the field is the wrong type, over-length,
// or contains abnormal control characters.
function locationString(value: unknown, maxBytes: number): string | null {
  if (value === undefined || value === null) return ''
  if (typeof value !== 'string') return null
  if (hasAbnormalControlCharacters(value)) return null
  const trimmed = value.trim()
  if (utf8Encoder.encode(trimmed).byteLength > maxBytes) return null
  return trimmed
}

// hasAbnormalControlCharacters rejects C0/C1 controls and Unicode bidirectional
// controls that could break layout or be used for spoofing.
function hasAbnormalControlCharacters(value: string): boolean {
  for (let index = 0; index < value.length; index++) {
    const code = value.codePointAt(index) ?? 0
    if (code < 0x20 || (code >= 0x7f && code <= 0x9f)) return true
    switch (code) {
      case 0x061c: // Arabic Letter Mark
      case 0x200e: // LRM
      case 0x200f: // RLM
      case 0x202a: // LRE
      case 0x202b: // RLE
      case 0x202c: // PDF
      case 0x202d: // LRO
      case 0x202e: // RLO
      case 0x2028: // Line Separator
      case 0x2029: // Paragraph Separator
      case 0x2066: // LRI
      case 0x2067: // RLI
      case 0x2068: // FSI
      case 0x2069: // PDI
        return true
    }
  }
  return false
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
