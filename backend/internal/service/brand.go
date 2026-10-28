package service

import "strings"

// Brand of this deployment. The public settings endpoint (and every user-facing
// artifact built from the site name: page titles, signup / verification /
// password-reset e-mails, the TOTP issuer, the payment subject shown on the
// provider's checkout page) publishes this name, so it lives in exactly one
// place.
const (
	// DefaultSiteName is the brand this deployment sells under.
	DefaultSiteName = "BestAIPrices"

	// LegacySiteName is the upstream default. Installations that ran the
	// original setup wizard have it persisted as `site_name`, so it is treated
	// as "not configured" and those deployments publish the brand without a
	// manual database edit.
	LegacySiteName = "Sub2API"
)

// NormalizeSiteName returns the name to publish for a stored `site_name` value.
// An empty value and the legacy upstream default both resolve to the brand; any
// other (admin-configured) value is returned trimmed, unchanged.
func NormalizeSiteName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" || name == LegacySiteName {
		return DefaultSiteName
	}
	return name
}
