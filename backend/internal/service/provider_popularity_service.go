package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/redis/go-redis/v9"
)

var defaultBotRegex = regexp.MustCompile(`(?i)(bot|spider|crawl|scraper|curl|wget|python-requests|headless|httpclient|postman|go-http-client)`)

const (
	DefaultClickCooldownDuration = 15 * time.Minute
	DefaultIPRateLimitWindow     = 1 * time.Minute
	DefaultMaxClicksPerIPPerMin  = 30
)

type RecordClickInput struct {
	Provider  string `json:"provider"`
	ClientIP  string `json:"client_ip"`
	UserID    int64  `json:"user_id"`
	UserAgent string `json:"user_agent"`
}

type ProviderPopularityService struct {
	repo        ProviderPopularityRepository
	redisClient *redis.Client
	cooldown    time.Duration
	maxIPPerMin int
	mu          sync.RWMutex
	memoryTTL   map[string]time.Time
	ipWindowMap map[string][]time.Time
}

func NewProviderPopularityService(repo ProviderPopularityRepository, redisClient *redis.Client) *ProviderPopularityService {
	return &ProviderPopularityService{
		repo:        repo,
		redisClient: redisClient,
		cooldown:    DefaultClickCooldownDuration,
		maxIPPerMin: DefaultMaxClicksPerIPPerMin,
		memoryTTL:   make(map[string]time.Time),
		ipWindowMap: make(map[string][]time.Time),
	}
}

// SetCooldown sets custom deduplication cooldown duration (primarily for testing)
func (s *ProviderPopularityService) SetCooldown(d time.Duration) {
	s.cooldown = d
}

// SetMaxClicksPerIPPerMin sets custom rate limit threshold (primarily for testing)
func (s *ProviderPopularityService) SetMaxClicksPerIPPerMin(max int) {
	s.maxIPPerMin = max
}

func (s *ProviderPopularityService) isBot(userAgent string) bool {
	cleanUA := strings.TrimSpace(userAgent)
	if cleanUA == "" {
		return true
	}
	return defaultBotRegex.MatchString(cleanUA)
}

func (s *ProviderPopularityService) checkIPRateLimit(ctx context.Context, clientIP string) bool {
	if clientIP == "" {
		return true
	}

	if s.redisClient != nil {
		key := fmt.Sprintf("provider_click:ip_rate:%s", clientIP)
		count, err := s.redisClient.Incr(ctx, key).Result()
		if err == nil {
			if count == 1 {
				_ = s.redisClient.Expire(ctx, key, DefaultIPRateLimitWindow).Err()
			}
			return count <= int64(s.maxIPPerMin)
		}
	}

	// In-memory rate limiting fallback
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-DefaultIPRateLimitWindow)

	var valid []time.Time
	for _, t := range s.ipWindowMap[clientIP] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	if len(valid) >= s.maxIPPerMin {
		s.ipWindowMap[clientIP] = valid
		return false
	}

	s.ipWindowMap[clientIP] = append(valid, now)
	return true
}

func (s *ProviderPopularityService) isCooldownActive(ctx context.Context, provider, clientIP string, userID int64) bool {
	keys := make([]string, 0, 2)
	if clientIP != "" {
		keys = append(keys, fmt.Sprintf("provider_click:cd:%s:%s", provider, clientIP))
	}
	if userID > 0 {
		keys = append(keys, fmt.Sprintf("provider_click:cd:%s:u:%d", provider, userID))
	}

	if len(keys) == 0 {
		return false
	}

	if s.redisClient != nil {
		for _, key := range keys {
			exists, err := s.redisClient.Exists(ctx, key).Result()
			if err == nil && exists > 0 {
				return true
			}
		}
	}

	// In-memory fallback
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, key := range keys {
		if exp, found := s.memoryTTL[key]; found {
			if exp.After(now) {
				return true
			}
			delete(s.memoryTTL, key)
		}
	}
	return false
}

func (s *ProviderPopularityService) markCooldown(ctx context.Context, provider, clientIP string, userID int64) {
	keys := make([]string, 0, 2)
	if clientIP != "" {
		keys = append(keys, fmt.Sprintf("provider_click:cd:%s:%s", provider, clientIP))
	}
	if userID > 0 {
		keys = append(keys, fmt.Sprintf("provider_click:cd:%s:u:%d", provider, userID))
	}

	if len(keys) == 0 {
		return
	}

	if s.redisClient != nil {
		for _, key := range keys {
			_ = s.redisClient.Set(ctx, key, "1", s.cooldown).Err()
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	exp := time.Now().Add(s.cooldown)
	for _, key := range keys {
		s.memoryTTL[key] = exp
	}
}

// RecordClick processes an AI provider click with anti-abuse verification
func (s *ProviderPopularityService) RecordClick(ctx context.Context, input RecordClickInput) (*RecordClickResult, error) {
	cleanProvider, err := domain.SanitizeProviderSlug(input.Provider)
	if err != nil {
		return nil, err
	}

	// 1. Bot & Crawler Protection
	if s.isBot(input.UserAgent) {
		return nil, ErrBotDetected
	}

	// 2. Global IP Rate Limiter
	if !s.checkIPRateLimit(ctx, input.ClientIP) {
		return nil, ErrClickRateLimitExceeded
	}

	// 3. Deduplication Cooldown Check
	if s.isCooldownActive(ctx, cleanProvider, input.ClientIP, input.UserID) {
		// Return current click count without incrementing
		current, err := s.repo.GetByProvider(ctx, cleanProvider)
		if err != nil {
			return nil, err
		}
		return &RecordClickResult{
			Provider:   cleanProvider,
			ClickCount: current.ClickCount,
			Counted:    false,
			Cooldown:   true,
		}, nil
	}

	// 4. Atomic Increment in DB
	res, err := s.repo.IncrementClick(ctx, cleanProvider)
	if err != nil {
		return nil, err
	}

	// 5. Activate Cooldown
	s.markCooldown(ctx, cleanProvider, input.ClientIP, input.UserID)

	return &RecordClickResult{
		Provider:   cleanProvider,
		ClickCount: res.ClickCount,
		Counted:    true,
		Cooldown:   false,
	}, nil
}

// GetPopularity returns popularity stats for a specific provider
func (s *ProviderPopularityService) GetPopularity(ctx context.Context, provider string) (*ProviderPopularity, error) {
	cleanProvider, err := domain.SanitizeProviderSlug(provider)
	if err != nil {
		return nil, err
	}
	return s.repo.GetByProvider(ctx, cleanProvider)
}

// ListPopularity returns all providers sorted by popularity rank
func (s *ProviderPopularityService) ListPopularity(ctx context.Context) ([]ProviderPopularityItem, error) {
	return s.repo.ListPopularity(ctx)
}
