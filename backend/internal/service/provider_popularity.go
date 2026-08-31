package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

var (
	ErrInvalidProviderName   = domain.ErrInvalidProviderName
	ErrClickRateLimitExceeded = domain.ErrClickRateLimitExceeded
	ErrBotDetected           = domain.ErrBotDetected
)

type ProviderPopularity = domain.ProviderPopularity
type ProviderPopularityItem = domain.ProviderPopularityItem
type RecordClickResult = domain.RecordClickResult

type ProviderPopularityRepository interface {
	IncrementClick(ctx context.Context, provider string) (*ProviderPopularity, error)
	GetByProvider(ctx context.Context, provider string) (*ProviderPopularity, error)
	ListPopularity(ctx context.Context) ([]ProviderPopularityItem, error)
}
