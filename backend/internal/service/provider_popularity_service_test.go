package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProviderPopularityRepo struct {
	counts map[string]int64
	times  map[string]time.Time
}

func newMockProviderPopularityRepo() *mockProviderPopularityRepo {
	return &mockProviderPopularityRepo{
		counts: make(map[string]int64),
		times:  make(map[string]time.Time),
	}
}

func (m *mockProviderPopularityRepo) IncrementClick(ctx context.Context, provider string) (*service.ProviderPopularity, error) {
	m.counts[provider]++
	now := time.Now()
	m.times[provider] = now
	return &service.ProviderPopularity{
		Provider:      provider,
		ClickCount:    m.counts[provider],
		LastClickedAt: &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func (m *mockProviderPopularityRepo) GetByProvider(ctx context.Context, provider string) (*service.ProviderPopularity, error) {
	cnt := m.counts[provider]
	t := m.times[provider]
	var lastClicked *time.Time
	if !t.IsZero() {
		lastClicked = &t
	}
	return &service.ProviderPopularity{
		Provider:      provider,
		ClickCount:    cnt,
		LastClickedAt: lastClicked,
	}, nil
}

func (m *mockProviderPopularityRepo) ListPopularity(ctx context.Context) ([]service.ProviderPopularityItem, error) {
	var items []service.ProviderPopularityItem
	rank := 1
	for p, cnt := range m.counts {
		t := m.times[p]
		var lastClicked *time.Time
		if !t.IsZero() {
			lastClicked = &t
		}
		items = append(items, service.ProviderPopularityItem{
			Provider:      p,
			ClickCount:    cnt,
			Rank:          rank,
			LastClickedAt: lastClicked,
		})
		rank++
	}
	return items, nil
}

func TestProviderPopularityService_RecordClick_SuccessAndCooldown(t *testing.T) {
	repo := newMockProviderPopularityRepo()
	svc := service.NewProviderPopularityService(repo, nil)
	svc.SetCooldown(100 * time.Millisecond)

	ctx := context.Background()
	validUA := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

	// 1. First click: counted
	res1, err := svc.RecordClick(ctx, service.RecordClickInput{
		Provider:  "openai",
		ClientIP:  "192.168.1.10",
		UserID:    101,
		UserAgent: validUA,
	})
	require.NoError(t, err)
	assert.Equal(t, "openai", res1.Provider)
	assert.Equal(t, int64(1), res1.ClickCount)
	assert.True(t, res1.Counted)
	assert.False(t, res1.Cooldown)

	// 2. Second click immediately from same IP/User: cooldown deduplication
	res2, err := svc.RecordClick(ctx, service.RecordClickInput{
		Provider:  "openai",
		ClientIP:  "192.168.1.10",
		UserID:    101,
		UserAgent: validUA,
	})
	require.NoError(t, err)
	assert.Equal(t, "openai", res2.Provider)
	assert.Equal(t, int64(1), res2.ClickCount) // Did not increment!
	assert.False(t, res2.Counted)
	assert.True(t, res2.Cooldown)

	// 3. Different provider from same IP: counted
	resClaude, err := svc.RecordClick(ctx, service.RecordClickInput{
		Provider:  "claude",
		ClientIP:  "192.168.1.10",
		UserID:    101,
		UserAgent: validUA,
	})
	require.NoError(t, err)
	assert.Equal(t, "claude", resClaude.Provider)
	assert.Equal(t, int64(1), resClaude.ClickCount)
	assert.True(t, resClaude.Counted)

	// 4. Wait for cooldown to expire on openai
	time.Sleep(120 * time.Millisecond)
	res3, err := svc.RecordClick(ctx, service.RecordClickInput{
		Provider:  "openai",
		ClientIP:  "192.168.1.10",
		UserID:    101,
		UserAgent: validUA,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), res3.ClickCount)
	assert.True(t, res3.Counted)
}

func TestProviderPopularityService_RecordClick_BotDetection(t *testing.T) {
	repo := newMockProviderPopularityRepo()
	svc := service.NewProviderPopularityService(repo, nil)

	ctx := context.Background()

	botUAs := []string{
		"curl/7.68.0",
		"python-requests/2.25.1",
		"Wget/1.20.3 (linux-gnu)",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"PostmanRuntime/7.28.4",
		"",
	}

	for _, ua := range botUAs {
		_, err := svc.RecordClick(ctx, service.RecordClickInput{
			Provider:  "openai",
			ClientIP:  "10.0.0.1",
			UserAgent: ua,
		})
		assert.ErrorIs(t, err, domain.ErrBotDetected, "expected bot detection for UA: %s", ua)
	}
}

func TestProviderPopularityService_RecordClick_RateLimit(t *testing.T) {
	repo := newMockProviderPopularityRepo()
	svc := service.NewProviderPopularityService(repo, nil)
	svc.SetMaxClicksPerIPPerMin(3)
	svc.SetCooldown(1 * time.Millisecond)

	ctx := context.Background()
	validUA := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0 Safari/537.36"

	// 3 clicks should succeed
	for i := 1; i <= 3; i++ {
		time.Sleep(2 * time.Millisecond)
		_, err := svc.RecordClick(ctx, service.RecordClickInput{
			Provider:  "gemini",
			ClientIP:  "203.0.113.1",
			UserAgent: validUA,
		})
		require.NoError(t, err)
	}

	// 4th click within 1 min exceeds rate limit
	_, err := svc.RecordClick(ctx, service.RecordClickInput{
		Provider:  "gemini",
		ClientIP:  "203.0.113.1",
		UserAgent: validUA,
	})
	assert.ErrorIs(t, err, domain.ErrClickRateLimitExceeded)
}

func TestProviderPopularityService_RecordClick_InvalidProvider(t *testing.T) {
	repo := newMockProviderPopularityRepo()
	svc := service.NewProviderPopularityService(repo, nil)

	ctx := context.Background()
	validUA := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0"

	invalidNames := []string{
		"",
		"   ",
		"provider with spaces",
		"provider<script>",
		"provider/../../etc",
	}

	for _, name := range invalidNames {
		_, err := svc.RecordClick(ctx, service.RecordClickInput{
			Provider:  name,
			ClientIP:  "127.0.0.1",
			UserAgent: validUA,
		})
		assert.ErrorIs(t, err, domain.ErrInvalidProviderName)
	}

	// Valid domain provider like chatfire.cn
	resDomain, err := svc.RecordClick(ctx, service.RecordClickInput{
		Provider:  "chatfire.cn",
		ClientIP:  "127.0.0.1",
		UserAgent: validUA,
	})
	require.NoError(t, err)
	assert.Equal(t, "chatfire.cn", resDomain.Provider)
}

func TestProviderPopularityService_GetAndListPopularity(t *testing.T) {
	repo := newMockProviderPopularityRepo()
	svc := service.NewProviderPopularityService(repo, nil)

	ctx := context.Background()
	validUA := "Mozilla/5.0 Chrome/120.0.0.0"

	_, err := svc.RecordClick(ctx, service.RecordClickInput{
		Provider:  "openai",
		ClientIP:  "1.1.1.1",
		UserAgent: validUA,
	})
	require.NoError(t, err)

	pop, err := svc.GetPopularity(ctx, "openai")
	require.NoError(t, err)
	assert.Equal(t, int64(1), pop.ClickCount)

	list, err := svc.ListPopularity(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "openai", list[0].Provider)
}
