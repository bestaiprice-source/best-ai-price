//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The admin form offers the provider's OWN group names (the ones a key can be
// issued in), so the group is picked instead of typed — and the ratio shown is
// the one that will actually be charged.
func TestAvailableBillingGroups(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"model_name": "m1", "model_ratio": 1.0, "enable_groups": []string{"cheap", "official-only"}},
				{"model_name": "m2", "model_ratio": 1.0, "enable_groups": []string{"pricey"}},
			},
			"group_ratio": map[string]float64{"cheap": 0.1, "pricey": 0.9},
		})
	}))
	defer ts.Close()

	svc := NewYunwuPricingServiceWithClient(ts.URL, ts.Client())
	groups := svc.AvailableBillingGroups(context.Background(), "openlux")
	require.NotEmpty(t, groups)

	names := make([]string, 0, len(groups))
	byName := make(map[string]ProviderBillingGroup, len(groups))
	for _, g := range groups {
		names = append(names, g.Name)
		byName[g.Name] = g
	}
	// Cheapest first; a group the ratio map omits bills at the official rate and
	// is offered as 1.0 (never hidden); `default` is always present.
	assert.Equal(t, []string{"cheap", "pricey", "official-only", "default"}, names)
	assert.InDelta(t, 0.1, byName["cheap"].Ratio, 1e-9)
	assert.InDelta(t, 0.9, byName["pricey"].Ratio, 1e-9)
	assert.InDelta(t, 1.0, byName["official-only"].Ratio, 1e-9)
	assert.InDelta(t, 1.0, byName["default"].Ratio, 1e-9)
	assert.Equal(t, 1, byName["official-only"].Models, "the dropdown shows how many models a group can serve")
}

func TestAvailableBillingGroupsUnknownProvider(t *testing.T) {
	svc := NewYunwuPricingServiceWithClient("", nil)
	assert.Empty(t, svc.AvailableBillingGroups(context.Background(), "no-such-provider"))
}
