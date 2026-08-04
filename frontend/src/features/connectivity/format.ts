import type { ConnectivityClientLocation, ConnectivityEndpointResult } from './types'

export type NetworkExitSummary =
  | { mode: 'common'; ip: string; location: ConnectivityClientLocation | null }
  | { mode: 'split' }
  | { mode: 'unknown' }
  | { mode: 'none' }

// formatTypicalLatency returns the integer millisecond value for the median of
// successful samples, or null when there is no usable latency. The caller
// decides the surrounding label ("典型延迟" vs "暂无可用延迟").
export function formatTypicalLatency(medianMs: number | null | undefined): number | null {
  if (medianMs === null || medianMs === undefined) return null
  if (!Number.isFinite(medianMs) || medianMs < 0) return null
  return Math.round(medianMs)
}

// formatClientLocation joins the non-empty country / region / city parts with
// " · ". When a localized country name is absent it falls back to the ISO
// country code, so a country-code-only database entry still renders as a
// region. Returns "" only when every part is empty (caller shows "地区未知").
export function formatClientLocation(location: ConnectivityClientLocation | null): string {
  if (!location) return ''
  const country = location.country.trim() !== '' ? location.country : location.country_code.trim()
  const parts = [country, location.region, location.city].filter((part) => part !== '')
  return parts.join(' · ')
}

// summarizeNetworkExit consolidates the per-endpoint verified egress IPs into a
// single display decision:
//   - none:    IP display disabled, no run results, OR the run did not complete.
//     An incomplete run must not render any egress banner at all.
//   - common:  every endpoint is graded and carries the same non-empty IP →
//     show the egress once at the top.
//   - split:   every endpoint is graded and there are two or more distinct
//     non-empty IPs → show the split hint and each row's own IP.
//   - unknown: every endpoint is graded but at least one verified IP is missing,
//     so no egress may be claimed → show "egress could not be identified",
//     never a guessed address.
export function summarizeNetworkExit(
  enabled: boolean,
  results: ConnectivityEndpointResult[],
): NetworkExitSummary {
  if (!enabled || results.length === 0) return { mode: 'none' }

  // An incomplete/cancelled/rate-limited endpoint means the whole run is not
  // finished; the design requires no egress banner on unfinished runs.
  if (results.some((item) => item.status !== 'graded')) return { mode: 'none' }

  // All endpoints graded. A missing verified IP on any endpoint makes the
  // egress unverifiable (show "无法识别"); only when every endpoint has an IP
  // can we claim common or split.
  if (results.some((item) => !item.clientIP)) return { mode: 'unknown' }

  const addresses = new Set(results.map((item) => item.clientIP as string))
  if (addresses.size === 1) {
    const ip = [...addresses][0]
    return { mode: 'common', ip, location: commonLocation(results, ip) }
  }
  return { mode: 'split' }
}

// commonLocation returns a region only when every graded endpoint carrying the
// common IP agrees on the same location (and none is missing). Disagreement
// keeps the IP visible while hiding the region.
function commonLocation(results: ConnectivityEndpointResult[], ip: string): ConnectivityClientLocation | null {
  const matches = results.filter((item) => item.status === 'graded' && item.clientIP === ip)
  if (matches.length === 0 || matches.some((item) => !item.clientLocation)) return null
  const first = matches[0].clientLocation!
  return matches.every((item) => sameLocation(item.clientLocation!, first)) ? first : null
}

function sameLocation(left: ConnectivityClientLocation, right: ConnectivityClientLocation): boolean {
  return left.country_code === right.country_code
    && left.country === right.country
    && left.region === right.region
    && left.city === right.city
}
