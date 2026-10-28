//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type settingPublicRepoStub struct {
	values map[string]string
}

func (s *settingPublicRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingPublicRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingPublicRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingPublicRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *settingPublicRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingPublicRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingPublicRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingService_GetPublicSettings_ExposesRegistrationEmailSuffixWhitelist(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyRegistrationEnabled:              "true",
			SettingKeyEmailVerifyEnabled:               "true",
			SettingKeyRegistrationEmailSuffixWhitelist: `["@EXAMPLE.com"," @foo.bar ","@invalid_domain",""]`,
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"@example.com", "@foo.bar"}, settings.RegistrationEmailSuffixWhitelist)
}

func TestSettingService_GetPublicSettings_SanitizesHomeContent(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyHomeContent: `<div onclick="steal()">hello</div><script>alert(1)</script><a href="javascript:alert(1)">x</a>`,
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.NotContains(t, settings.HomeContent, "<script")
	require.NotContains(t, settings.HomeContent, "onclick=")
	require.NotContains(t, strings.ToLower(settings.HomeContent), "javascript:")
	require.Contains(t, settings.HomeContent, "hello")
}

func TestSettingService_GetPublicSettings_AllowsHTTPEmbeddedURL(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyPurchaseSubscriptionEnabled: "true",
			SettingKeyPurchaseSubscriptionURL:     "http://pay.example.com/checkout",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.PurchaseSubscriptionEnabled)
	require.Equal(t, "http://pay.example.com/checkout", settings.PurchaseSubscriptionURL)
}

// The brand is published as "Best Ai Price". An installation set up by the
// upstream wizard has `site_name = "Sub2API"` persisted, so that legacy value is
// treated as "not configured" and must not leak into the public settings.
func TestSettingService_GetPublicSettings_PublishesTheBrand(t *testing.T) {
	cases := []struct {
		name  string
		store map[string]string
		want  string
	}{
		{name: "legacy upstream default is replaced", store: map[string]string{SettingKeySiteName: "Sub2API"}, want: DefaultSiteName},
		{name: "blank value falls back to the brand", store: map[string]string{SettingKeySiteName: "   "}, want: DefaultSiteName},
		{name: "unset falls back to the brand", store: map[string]string{}, want: DefaultSiteName},
		{name: "configured value wins", store: map[string]string{SettingKeySiteName: "My Gateway"}, want: "My Gateway"},
		{name: "surrounding whitespace is trimmed", store: map[string]string{SettingKeySiteName: "  My Gateway  "}, want: "My Gateway"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewSettingService(&settingPublicRepoStub{values: tc.store}, &config.Config{})

			settings, err := svc.GetPublicSettings(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.want, settings.SiteName)
			require.NotEqual(t, LegacySiteName, settings.SiteName)
		})
	}
}
