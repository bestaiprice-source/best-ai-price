/**
 * Brand of this deployment.
 *
 * The published site name is resolved here so page titles, the header/sidebar
 * brand, auth screens and (through the backend's site_name setting) e-mails,
 * the TOTP issuer and payment subjects all carry one string.
 */
export const BRAND_NAME = 'BestAIPrices'

/**
 * Upstream default persisted by older installations (`site_name = "Sub2API"`).
 * It is treated as "not configured" so those deployments publish the brand
 * without a manual settings edit.
 */
export const LEGACY_BRAND_NAME = 'Sub2API'

/** Resolves the name to display for a raw `site_name` value. */
export function resolveBrandName(name?: string | null): string {
  const trimmed = (name ?? '').trim()
  return !trimmed || trimmed === LEGACY_BRAND_NAME ? BRAND_NAME : trimmed
}
