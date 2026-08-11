type BrowserLocation = Pick<Location, 'hostname' | 'origin'>

type PublicEndpoint = {
  documentationOrigin: string
}

const endpointByHost: Record<string, PublicEndpoint> = {
  'cn2.ppxcode.com': { documentationOrigin: 'https://doc.ppxcode.com' },
  'cf.ppxcode.com': { documentationOrigin: 'https://doc.ppxcode.com' },
}

function currentLocation(): BrowserLocation | undefined {
  return typeof window === 'undefined' ? undefined : window.location
}

function endpointFor(location = currentLocation()): PublicEndpoint | undefined {
  return location ? endpointByHost[location.hostname.toLowerCase()] : undefined
}

function parseHttpUrl(value: string): URL | undefined {
  try {
    const url = new URL(value.trim())
    return url.protocol === 'http:' || url.protocol === 'https:' ? url : undefined
  } catch {
    return undefined
  }
}

/**
 * Uses the visitor's selected public entry point only for known edge domains.
 * Canonical settings remain untouched for server-side callbacks and integrations.
 */
export function resolvePublicApiBaseUrl(configuredBaseUrl: string, location = currentLocation()): string {
  return endpointFor(location) ? location!.origin : configuredBaseUrl
}

/**
 * Routes only the canonical documentation host to its matching edge documentation site.
 */
export function resolvePublicDocumentationUrl(configuredUrl: string, location = currentLocation()): string {
  const endpoint = endpointFor(location)
  const configured = parseHttpUrl(configuredUrl)
  if (!endpoint || !configured || configured.hostname !== 'doc.psydo.top') return configuredUrl

  const resolved = new URL(configured.toString())
  resolved.href = `${endpoint.documentationOrigin}${configured.pathname}${configured.search}${configured.hash}`
  return resolved.toString()
}

/**
 * Rewrites only the configured canonical /home URL. Arbitrary administrator content stays intact.
 */
export function resolvePublicHomeContent(
  configuredContent: string,
  configuredApiBaseUrl: string,
  location = currentLocation(),
): string {
  const endpoint = endpointFor(location)
  const content = parseHttpUrl(configuredContent)
  const configuredApi = parseHttpUrl(configuredApiBaseUrl)
  if (!endpoint || !content || !configuredApi || content.origin !== configuredApi.origin || !['/home', '/home/'].includes(content.pathname)) {
    return configuredContent
  }

  const resolved = new URL(content.toString())
  resolved.href = `${location!.origin}${content.pathname}${content.search}${content.hash}`
  return resolved.toString()
}
