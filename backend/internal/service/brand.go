package service

import "strings"

// Brand of this deployment. The public settings endpoint (and every user-facing
// artifact built from the site name: page titles, signup / verification /
// password-reset e-mails, the TOTP issuer, the payment subject shown on the
// provider's checkout page) publishes this name, so it lives in exactly one
// place.
const (
	// DefaultSiteName is the brand this deployment sells under.
	DefaultSiteName = "BestAIPrice"

	// LegacySiteName is the upstream default. Installations that ran the
	// original setup wizard have it persisted as `site_name`, so it is treated
	// as "not configured" and those deployments publish the brand without a
	// manual database edit.
	LegacySiteName = "Sub2API"

	// LegacyBrandSpelling is the previous spelling of our own brand. An
	// installation that saved it has it persisted as `site_name`; like the
	// upstream default it is treated as "not configured" so the corrected
	// spelling publishes without a manual database edit.
	LegacyBrandSpelling = "BestAIPrices"

	// DefaultSiteSubtitle is the tagline this deployment publishes.
	DefaultSiteSubtitle = "AI API gateway — Claude, GPT, Gemini, Kimi and more behind one key"

	// LegacySiteSubtitle is the upstream fork's tagline ("Subscription to API
	// Conversion Platform"). A wizard-seeded installation has it persisted, so
	// it is treated as "not configured" for the same reason as LegacySiteName.
	LegacySiteSubtitle = "Subscription to API Conversion Platform"
)

// NormalizeSiteSubtitle returns the subtitle to publish for a stored
// `site_subtitle` value. An empty value and the legacy upstream tagline both
// resolve to the deployment's own subtitle; any other admin-configured value is
// returned trimmed, unchanged.
func NormalizeSiteSubtitle(raw string) string {
	subtitle := strings.TrimSpace(raw)
	if subtitle == "" || subtitle == LegacySiteSubtitle {
		return DefaultSiteSubtitle
	}
	return subtitle
}

// NormalizeSiteName returns the name to publish for a stored `site_name` value.
// An empty value, the legacy upstream default and the previous spelling of our
// own brand all resolve to the brand; any other admin-configured value is
// returned trimmed, unchanged.
func NormalizeSiteName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" || name == LegacySiteName || name == LegacyBrandSpelling {
		return DefaultSiteName
	}
	return name
}
