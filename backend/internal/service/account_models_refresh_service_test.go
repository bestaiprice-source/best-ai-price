//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type accountModelsRefreshRepoStub struct {
	accountRepoStub
	accounts       map[int64]*Account
	updateExtraIDs []int64
	mu             sync.Mutex
}

func (s *accountModelsRefreshRepoStub) ListActive(_ context.Context) ([]Account, error) {
	out := make([]Account, 0, len(s.accounts))
	for _, account := range s.accounts {
		if account == nil {
			continue
		}
		cloned := *account
		out = append(out, cloned)
	}
	return out, nil
}

func (s *accountModelsRefreshRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	account, ok := s.accounts[id]
	if !ok || account == nil {
		return nil, fmt.Errorf("account %d not found", id)
	}
	cloned := *account
	return &cloned, nil
}

func (s *accountModelsRefreshRepoStub) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.updateExtraIDs = append(s.updateExtraIDs, id)
	account := s.accounts[id]
	if account == nil {
		return nil
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any, len(updates))
	}
	for key, value := range updates {
		account.Extra[key] = value
	}
	return nil
}

func TestAccountModelsRefreshService_RunOnceRefreshesOnlyDueSupportedAccounts(t *testing.T) {
	now := time.Now().UTC()
	repo := &accountModelsRefreshRepoStub{
		accounts: map[int64]*Account{
			1: {
				ID:       1,
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Status:   StatusActive,
				Credentials: map[string]any{
					"api_key": "sk-test",
				},
				Extra: map[string]any{
					AccountExtraModelsRefreshIntervalSecKey: 60,
					AccountExtraModelsFetchedAtKey:          now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
				},
			},
			2: {
				ID:       2,
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Status:   StatusActive,
				Credentials: map[string]any{
					"api_key": "sk-fresh",
				},
				Extra: map[string]any{
					AccountExtraModelsRefreshIntervalSecKey: 3600,
					AccountExtraModelsFetchedAtKey:          now.Format(time.RFC3339Nano),
				},
			},
			3: {
				ID:       3,
				Platform: PlatformSora,
				Type:     AccountTypeOAuth,
				Status:   StatusActive,
				Extra: map[string]any{
					AccountExtraModelsRefreshIntervalSecKey: 60,
				},
			},
		},
	}
	upstream := &modelFetchHTTPUpstreamStub{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"gpt-5"},{"id":"gpt-5-mini"}]}`)),
		},
	}
	accountTestSvc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
	}
	svc := NewAccountModelsRefreshService(repo, accountTestSvc, time.Minute)

	svc.runOnce()

	require.Equal(t, []int64{1}, repo.updateExtraIDs)
	require.NotNil(t, repo.accounts[1].Extra[AccountExtraFetchedModelsKey])
	require.Nil(t, repo.accounts[2].Extra[AccountExtraFetchedModelsKey])
	require.Nil(t, repo.accounts[3].Extra[AccountExtraFetchedModelsKey])
}

func TestShouldRefreshFetchedModels_DefaultsToHourlyForSupportedPlatforms(t *testing.T) {
	now := time.Now().UTC()
	fetchedAgo := func(d time.Duration) map[string]any {
		return map[string]any{AccountExtraModelsFetchedAtKey: now.Add(-d).Format(time.RFC3339Nano)}
	}

	tests := []struct {
		name    string
		account Account
		want    bool
	}{
		{
			// The catalog's per-model evidence is built from this list, and the
			// fetch is a free GET, so an unset interval must not opt an account
			// out entirely.
			name:    "yunwu without interval and never fetched",
			account: Account{Platform: PlatformYunwu},
			want:    true,
		},
		{
			name:    "yunwu without interval, fetched 30 minutes ago",
			account: Account{Platform: PlatformYunwu, Extra: fetchedAgo(30 * time.Minute)},
			want:    false,
		},
		{
			name:    "yunwu without interval, fetched two hours ago",
			account: Account{Platform: PlatformYunwu, Extra: fetchedAgo(2 * time.Hour)},
			want:    true,
		},
		{
			name: "explicit interval still wins over the default",
			account: Account{
				Platform: PlatformYunwu,
				Extra: map[string]any{
					AccountExtraModelsRefreshIntervalSecKey: 60,
					AccountExtraModelsFetchedAtKey:          now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
				},
			},
			want: true,
		},
		{
			// Unsupported platform: stays opted out so we never write a refresh
			// error for an account whose list cannot be fetched.
			name:    "sora without interval",
			account: Account{Platform: PlatformSora},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.account.ShouldRefreshFetchedModels(now))
		})
	}
}

func TestAccountModelsRefreshService_RunOnceSkipsConcurrentRun(t *testing.T) {
	repo := &accountModelsRefreshRepoStub{
		accounts: map[int64]*Account{},
	}
	accountTestSvc := &AccountTestService{
		accountRepo: repo,
	}
	svc := NewAccountModelsRefreshService(repo, accountTestSvc, time.Minute)
	svc.runInProgress.Store(true)

	svc.runOnce()

	require.Empty(t, repo.updateExtraIDs)
}
