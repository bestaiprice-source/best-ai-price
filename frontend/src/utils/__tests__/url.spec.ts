import { describe, it, expect, afterEach, vi } from 'vitest'
import { appPath, getBasePath, getPublicApiBaseUrl, getPublicAppBaseUrl } from '../url'

describe('getPublicAppBaseUrl', () => {
  const originalWindow = globalThis.window

  afterEach(() => {
    if (originalWindow) {
      Object.defineProperty(globalThis, 'window', {
        value: originalWindow,
        writable: true,
        configurable: true,
      })
    }
  })

  it('detects /gateway reverse-proxy subpath from pathname', () => {
    Object.defineProperty(globalThis, 'window', {
      value: {
        location: {
          origin: 'https://bestaiprice.com',
          pathname: '/gateway/purchase',
        } as unknown as Location,
      },
      writable: true,
      configurable: true,
    })

    expect(getPublicAppBaseUrl()).toBe('https://bestaiprice.com/gateway')
  })

  it('detects /sub2api reverse-proxy subpath before /payment', () => {
    Object.defineProperty(globalThis, 'window', {
      value: {
        location: {
          origin: 'https://example.com',
          pathname: '/sub2api/payment/result',
        } as unknown as Location,
      },
      writable: true,
      configurable: true,
    })

    expect(getPublicAppBaseUrl()).toBe('https://example.com/sub2api')
  })

  it('returns origin when running at root path', () => {
    Object.defineProperty(globalThis, 'window', {
      value: {
        location: {
          origin: 'https://sub2api.example.com',
          pathname: '/purchase',
        } as unknown as Location,
      },
      writable: true,
      configurable: true,
    })

    expect(getPublicAppBaseUrl()).toBe('https://sub2api.example.com')
  })
})

describe('getPublicApiBaseUrl', () => {
  const originalWindow = globalThis.window

  afterEach(() => {
    vi.unstubAllEnvs()
    if (originalWindow) {
      Object.defineProperty(globalThis, 'window', {
        value: originalWindow,
        writable: true,
        configurable: true,
      })
    }
  })

  const setLocation = (origin: string, pathname: string) => {
    Object.defineProperty(globalThis, 'window', {
      value: { location: { origin, pathname } as unknown as Location },
      writable: true,
      configurable: true,
    })
  }

  it('uses the configured api_base_url when the admin set one', () => {
    setLocation('https://bestaiprice.com', '/gateway/keys')
    vi.stubEnv('BASE_URL', '/gateway/')

    expect(getPublicApiBaseUrl('https://bestaiprice.com/gateway')).toBe('https://bestaiprice.com/gateway')
    expect(getPublicApiBaseUrl('https://bestaiprice.com/gateway/')).toBe('https://bestaiprice.com/gateway')
  })

  it('falls back to the app base INCLUDING the /gateway sub-path, never the bare origin', () => {
    setLocation('https://bestaiprice.com', '/gateway/docs')
    vi.stubEnv('BASE_URL', '/gateway/')

    // The bare origin serves the catalog site: /v1/chat/completions there answers with
    // an HTML 404, which is the bug this fallback exists to prevent.
    expect(getPublicApiBaseUrl('')).toBe('https://bestaiprice.com/gateway')
    expect(getPublicApiBaseUrl(undefined)).toBe('https://bestaiprice.com/gateway')
    expect(getPublicApiBaseUrl('   ')).toBe('https://bestaiprice.com/gateway')
    expect(getPublicApiBaseUrl()).not.toBe('https://bestaiprice.com')
  })

  it('does not double up slashes when the configured value carries a trailing one', () => {
    setLocation('https://bestaiprice.com', '/gateway/keys')
    vi.stubEnv('BASE_URL', '/gateway/')

    // The OpenAI base is built as `${base}/v1` everywhere.
    expect(`${getPublicApiBaseUrl('https://bestaiprice.com/gateway/')}/v1`).toBe('https://bestaiprice.com/gateway/v1')
  })
})

describe('appPath', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('prefixes an in-app path with the reverse-proxy base', () => {
    vi.stubEnv('BASE_URL', '/gateway/')

    expect(appPath('/purchase')).toBe('/gateway/purchase')
    expect(appPath('/orders')).toBe('/gateway/orders')
    expect(appPath('/admin/settings')).toBe('/gateway/admin/settings')
  })

  it('normalizes a base without a trailing slash and a path without a leading one', () => {
    vi.stubEnv('BASE_URL', '/gateway')

    expect(appPath('purchase')).toBe('/gateway/purchase')
  })

  it('stays a no-op when the app is served from the origin root', () => {
    vi.stubEnv('BASE_URL', '/')

    expect(appPath('/purchase')).toBe('/purchase')
  })

  it('never returns a root-relative path while a base is configured', () => {
    vi.stubEnv('BASE_URL', '/gateway/')

    // A bare '/purchase' would be resolved against the origin by the browser and
    // leave the SPA entirely — the bug this helper exists to prevent.
    expect(appPath('/purchase')).not.toBe('/purchase')
    expect(appPath('/purchase').startsWith(getBasePath())).toBe(true)
  })
})
