/**
 * Brand of this deployment.
 *
 * The published site name is resolved here so page titles, the header/sidebar
 * brand, auth screens and (through the backend's site_name setting) e-mails,
 * the TOTP issuer and payment subjects all carry one string.
 */
export const BRAND_NAME = 'BestAIPrice'

/**
 * Names treated as "not configured" so a deployment keeps publishing the brand
 * without a manual settings edit:
 *  - `Sub2API` — the upstream default persisted by wizard-seeded installations;
 *  - `BestAIPrices` — the previous spelling of our own brand, which is still
 *    stored as `site_name` on installations that saved it.
 */
export const LEGACY_BRAND_NAMES = ['Sub2API', 'BestAIPrices'] as const

/** Resolves the name to display for a raw `site_name` value. */
export function resolveBrandName(name?: string | null): string {
  const trimmed = (name ?? '').trim()
  return !trimmed || (LEGACY_BRAND_NAMES as readonly string[]).includes(trimmed) ? BRAND_NAME : trimmed
}
