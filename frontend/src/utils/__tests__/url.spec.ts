import { describe, it, expect, afterEach } from 'vitest'
import { getPublicAppBaseUrl } from '../url'

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
