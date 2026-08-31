package domain

import (
	"regexp"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrInvalidProviderName   = infraerrors.BadRequest("INVALID_PROVIDER_NAME", "provider name must be between 1 and 100 alphanumeric characters, hyphens or underscores")
	ErrClickRateLimitExceeded = infraerrors.TooManyRequests("CLICK_RATE_LIMIT_EXCEEDED", "too many click requests, please slow down")
	ErrBotDetected           = infraerrors.Forbidden("BOT_DETECTED", "automated bot traffic is not allowed")
)

var providerSlugRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,100}$`)

// SanitizeProviderSlug validates and cleans provider slug
func SanitizeProviderSlug(slug string) (string, error) {
	clean := strings.TrimSpace(slug)
	if clean == "" || len(clean) > 100 || !providerSlugRegex.MatchString(clean) {
		return "", ErrInvalidProviderName
	}
	return strings.ToLower(clean), nil
}

// ProviderPopularity represents popularity stats for an AI provider
type ProviderPopularity struct {
	ID            int64      `json:"id"`
	Provider      string     `json:"provider"`
	ClickCount    int64      `json:"click_count"`
	LastClickedAt *time.Time `json:"last_clicked_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ProviderPopularityItem represents an item in the popularity rankings
type ProviderPopularityItem struct {
	Provider      string     `json:"provider"`
	ClickCount    int64      `json:"click_count"`
	Rank          int        `json:"rank"`
	LastClickedAt *time.Time `json:"last_clicked_at,omitempty"`
}

// RecordClickResult represents the result of a recorded click attempt
type RecordClickResult struct {
	Provider   string `json:"provider"`
	ClickCount int64  `json:"click_count"`
	Counted    bool   `json:"counted"`          // true if counted, false if deduplicated in cooldown window
	Cooldown   bool   `json:"cooldown,omitempty"` // true if deduplicated due to cooldown window
}
