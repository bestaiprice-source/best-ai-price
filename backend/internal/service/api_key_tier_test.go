//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
	"github.com/stretchr/testify/require"
)

// tierAPIKeyRepoStub reuses the full APIKeyRepository stub (every unused method
// panics) and only implements the three calls these tests make.
type tierAPIKeyRepoStub struct {
	quotaBaseAPIKeyRepoStub

	key     *APIKey
	created *APIKey
	updated *APIKey
}

func (s *tierAPIKeyRepoStub) Create(_ context.Context, key *APIKey) error {
	clone := *key
	s.created = &clone
	return nil
}

func (s *tierAPIKeyRepoStub) GetByID(_ context.Context, _ int64) (*APIKey, error) {
	if s.key == nil {
		return nil, ErrAPIKeyNotFound
	}
	clone := *s.key
	return &clone, nil
}

func (s *tierAPIKeyRepoStub) Update(_ context.Context, key *APIKey) error {
	clone := *key
	s.updated = &clone
	return nil
}

func newTierTestService(repo *tierAPIKeyRepoStub, user *User) *APIKeyService {
	return &APIKeyService{
		apiKeyRepo: repo,
		userRepo:   &userRepoStub{user: user},
		cfg:        &config.Config{Default: config.DefaultConfig{APIKeyPrefix: "sk-"}},
	}
}

// ---------------------------------------------------------------------------
// Tier normalization and resolution
// ---------------------------------------------------------------------------

func TestNormalizeAPIKeyTier(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: ""},
		{in: "   ", want: ""},
		{in: "smart", want: "smart"},
		{in: "fast", want: "fast"},
		{in: " FAST ", want: "fast"},
		{in: "Smart", want: "smart"},
		{in: "turbo", wantErr: true},
		{in: "fastest", wantErr: true},
	}
	for _, tc := range cases {
		got, err := NormalizeAPIKeyTier(tc.in)
		if tc.wantErr {
			require.ErrorIsf(t, err, ErrInvalidAPIKeyTier, "input %q", tc.in)
			continue
		}
		require.NoErrorf(t, err, "input %q", tc.in)
		require.Equalf(t, tc.want, got, "input %q", tc.in)
	}
}

func TestEffectiveYunwuTier(t *testing.T) {
	t.Run("the tier stored on the key wins", func(t *testing.T) {
		key := &APIKey{Tier: "fast", Group: &Group{Name: "smart"}}
		require.Equal(t, "fast", EffectiveYunwuTier(key))
	})

	t.Run("an empty tier falls back to the group names", func(t *testing.T) {
		key := &APIKey{Tier: "", Group: &Group{Name: "Fast"}}
		require.Equal(t, "fast", EffectiveYunwuTier(key))
	})

	t.Run("no tier and no tier group is smart", func(t *testing.T) {
		key := &APIKey{Tier: "", Group: &Group{Name: "yunwu"}}
		require.Equal(t, "smart", EffectiveYunwuTier(key))
	})

	t.Run("nil key is smart", func(t *testing.T) {
		require.Equal(t, "smart", EffectiveYunwuTier(nil))
	})

	t.Run("the tier travels in the request context", func(t *testing.T) {
		ctx := WithYunwuTier(context.Background(), &APIKey{Tier: "fast"})
		require.Equal(t, "fast", yunwu.TierFromContext(ctx))
	})

	t.Run("a key without a tier stays on smart in the context", func(t *testing.T) {
		ctx := WithYunwuTier(context.Background(), &APIKey{Group: &Group{Name: "yunwu"}})
		require.Equal(t, "smart", yunwu.TierFromContext(ctx))
	})
}

// ---------------------------------------------------------------------------
// Create / Update persist the tier
// ---------------------------------------------------------------------------

func TestCreateAPIKeyStoresNormalizedTier(t *testing.T) {
	repo := &tierAPIKeyRepoStub{}
	svc := newTierTestService(repo, &User{ID: 7, Status: StatusActive, Role: "user"})

	created, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "fast key", Tier: " FAST "})
	require.NoError(t, err)
	require.Equal(t, "fast", created.Tier)
	require.NotNil(t, repo.created)
	require.Equal(t, "fast", repo.created.Tier, "the repository must persist the tier")
}

func TestCreateAPIKeyWithoutTierStaysEmpty(t *testing.T) {
	repo := &tierAPIKeyRepoStub{}
	svc := newTierTestService(repo, &User{ID: 7, Status: StatusActive, Role: "user"})

	created, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "legacy key"})
	require.NoError(t, err)
	require.Equal(t, "", created.Tier, "an empty tier keeps the legacy group-name derivation")
	require.NotNil(t, repo.created)
	require.Equal(t, "", repo.created.Tier)
}

func TestCreateAPIKeyRejectsAnUnknownTier(t *testing.T) {
	repo := &tierAPIKeyRepoStub{}
	svc := newTierTestService(repo, &User{ID: 7, Status: StatusActive, Role: "user"})

	_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "bad", Tier: "turbo"})
	require.ErrorIs(t, err, ErrInvalidAPIKeyTier)
	require.Nil(t, repo.created, "nothing may be persisted for an invalid tier")
}

func TestUpdateAPIKeyChangesTheTier(t *testing.T) {
	key := &APIKey{ID: 5, UserID: 7, Key: "sk-existing", Name: "key", Status: StatusActive}
	repo := &tierAPIKeyRepoStub{key: key}
	svc := newTierTestService(repo, &User{ID: 7, Status: StatusActive, Role: "user"})

	tier := "fast"
	updated, err := svc.Update(context.Background(), 5, 7, UpdateAPIKeyRequest{
		IPWhitelist: []string{},
		IPBlacklist: []string{},
		Tier:        &tier,
	})
	require.NoError(t, err)
	require.Equal(t, "fast", updated.Tier)
	require.NotNil(t, repo.updated)
	require.Equal(t, "fast", repo.updated.Tier)
}

func TestUpdateAPIKeyKeepsTheTierWhenItIsNotSent(t *testing.T) {
	key := &APIKey{ID: 5, UserID: 7, Key: "sk-existing", Name: "key", Status: StatusActive, Tier: "fast"}
	repo := &tierAPIKeyRepoStub{key: key}
	svc := newTierTestService(repo, &User{ID: 7, Status: StatusActive, Role: "user"})

	updated, err := svc.Update(context.Background(), 5, 7, UpdateAPIKeyRequest{
		IPWhitelist: []string{},
		IPBlacklist: []string{},
	})
	require.NoError(t, err)
	require.Equal(t, "fast", updated.Tier)
	require.Equal(t, "fast", repo.updated.Tier)
}

func TestUpdateAPIKeyRejectsAnUnknownTier(t *testing.T) {
	key := &APIKey{ID: 5, UserID: 7, Key: "sk-existing", Name: "key", Status: StatusActive, Tier: "fast"}
	repo := &tierAPIKeyRepoStub{key: key}
	svc := newTierTestService(repo, &User{ID: 7, Status: StatusActive, Role: "user"})

	tier := "turbo"
	_, err := svc.Update(context.Background(), 5, 7, UpdateAPIKeyRequest{
		IPWhitelist: []string{},
		IPBlacklist: []string{},
		Tier:        &tier,
	})
	require.ErrorIs(t, err, ErrInvalidAPIKeyTier)
	require.Nil(t, repo.updated, "an invalid tier must not be persisted")
}

// ---------------------------------------------------------------------------
// Auth cache carries the tier (the hot path never re-reads the row)
// ---------------------------------------------------------------------------

func TestAuthCacheSnapshotCarriesTheTier(t *testing.T) {
	svc := &APIKeyService{}
	key := &APIKey{
		ID:     3,
		UserID: 7,
		Tier:   "fast",
		Status: StatusActive,
		User:   &User{ID: 7, Status: StatusActive, Role: "user"},
	}

	snapshot := svc.snapshotFromAPIKey(key)
	require.NotNil(t, snapshot)
	require.Equal(t, "fast", snapshot.Tier)

	restored := svc.snapshotToAPIKey("sk-abc", snapshot)
	require.NotNil(t, restored)
	require.Equal(t, "fast", restored.Tier, "a cached key must keep its tier")
	require.Equal(t, "fast", EffectiveYunwuTier(restored))
}
