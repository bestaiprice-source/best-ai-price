package handler

import (
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Public projection of the pooled unofficial-provider platform ("yunwu").
//
// "yunwu" is an INTERNAL label: it names the pool of upstream reseller accounts
// the gateway schedules against, not anything the signed-in user chose. Every
// user-facing response must publish a neutral label instead, otherwise our
// infrastructure naming leaks to anyone holding an account
// (/api/v1/groups/available, /api/v1/channels/available).
//
// The projection lives here — one place, applied at the response boundary — so
// it cannot drift between the two endpoints, exactly like NormalizeSiteName
// resolves the brand for the settings endpoint. The stored value is untouched,
// so the admin side keeps seeing the real one.
const (
	// neutralPoolPlatform is published in place of the internal platform label.
	neutralPoolPlatform = "standard"

	// neutralPoolGroupName replaces a stored group name that names the pool.
	neutralPoolGroupName = "standard"
)

// internalPoolLabelPattern matches a stored group name that names the upstream
// pool (e.g. "yunwu", "Yunwu Pool", "yunwu-smart"). Names that do not name the
// pool — the billing tiers "smart" / "fast" that a provider issues its keys
// in, or anything an administrator typed — are published unchanged, because the
// user-facing tier picker matches on them.
var internalPoolLabelPattern = regexp.MustCompile(`(?i)\byunwu\b`)

// publicGroupPlatform resolves the platform label published to signed-in users.
// The internal pool label is replaced with a neutral one; every other platform
// (anthropic / openai / gemini / …) is published as-is.
func publicGroupPlatform(platform string) string {
	if strings.EqualFold(strings.TrimSpace(platform), service.PlatformYunwu) {
		return neutralPoolPlatform
	}
	return platform
}

// publicGroupName resolves the group name published to signed-in users. For a
// group on the internal pool platform the name is published unchanged unless it
// names the pool itself, in which case the neutral name is published instead.
func publicGroupName(platform, name string) string {
	if !strings.EqualFold(strings.TrimSpace(platform), service.PlatformYunwu) {
		return name
	}
	if internalPoolLabelPattern.MatchString(name) {
		return neutralPoolGroupName
	}
	return name
}

// isInternalPoolPlatform reports whether a stored platform label is the
// internal capacity pool (case-insensitive).
func isInternalPoolPlatform(platform string) bool {
	return strings.EqualFold(strings.TrimSpace(platform), service.PlatformYunwu)
}
