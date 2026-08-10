/**
 * Base URL and asset resolution utilities
 * Ensures all API calls, setup calls, and asset URLs respect Vite's BASE_URL (e.g. '/gateway/')
 */

/**
 * 验证并规范化 URL
 * 默认只接受绝对 URL（以 http:// 或 https:// 开头），可按需允许相对路径
 * @param value 用户输入的 URL
 * @returns 规范化后的 URL，如果无效则返回空字符串
 */
type SanitizeOptions = {
  allowRelative?: boolean
  allowDataUrl?: boolean
}

export function sanitizeUrl(value: string, options: SanitizeOptions = {}): string {
  const trimmed = value.trim()
  if (!trimmed) {
    return ''
  }

  if (options.allowRelative && trimmed.startsWith('/') && !trimmed.startsWith('//')) {
    return trimmed
  }

  // 允许 data:image/ 开头的 data URL（仅限图片类型）
  if (options.allowDataUrl && trimmed.startsWith('data:image/')) {
    return trimmed
  }

  // 只接受绝对 URL，不使用 base URL 来避免相对路径被解析为当前域名
  // 检查是否以 http:// 或 https:// 开头
  if (!trimmed.match(/^https?:\/\//i)) {
    return ''
  }

  try {
    const parsed = new URL(trimmed)
    const protocol = parsed.protocol.toLowerCase()
    if (protocol !== 'http:' && protocol !== 'https:') {
      return ''
    }
    return parsed.toString()
  } catch {
    return ''
  }
}

/**
 * Returns the base path configured for Vite (e.g. '/gateway/' or '/')
 */
export function getBasePath(): string {
  const base = import.meta.env.BASE_URL || '/'
  return base.endsWith('/') ? base : `${base}/`
}

/**
 * Returns the API base URL incorporating the base path if VITE_API_BASE_URL is not set.
 */
export function getApiBaseUrl(): string {
  if (import.meta.env.VITE_API_BASE_URL) {
    return import.meta.env.VITE_API_BASE_URL
  }
  return `${getBasePath()}api/v1`.replace(/\/+/g, '/')
}

/**
 * Returns the Setup base URL incorporating the base path if VITE_SETUP_BASE_URL is not set.
 */
export function getSetupBaseUrl(): string {
  if (import.meta.env.VITE_SETUP_BASE_URL) {
    return import.meta.env.VITE_SETUP_BASE_URL
  }
  return `${getBasePath()}setup`.replace(/\/+/g, '/')
}

/**
 * Resolves an asset path or logo URL against Vite's BASE_URL if relative.
 */
export function resolveAssetUrl(url?: string | null): string {
  const defaultLogo = `${getBasePath()}logo.png`.replace(/\/+/g, '/')
  if (!url) {
    return defaultLogo
  }
  if (url.startsWith('http://') || url.startsWith('https://') || url.startsWith('data:')) {
    return url
  }
  const cleanUrl = url.startsWith('/') ? url.slice(1) : url
  return `${getBasePath()}${cleanUrl}`.replace(/\/+/g, '/')
}
