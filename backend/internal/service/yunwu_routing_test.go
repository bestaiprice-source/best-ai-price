package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
)

func TestWithYunwuModelRouting(t *testing.T) {
	ctx := context.Background()

	// Disabled routing returns the original context unchanged.
	disabled := &Group{ModelRoutingEnabled: false, ModelRouting: map[string][]int64{"claude-3-7-sonnet": {7}}}
	if got := yunwu.ModelRoutingFromContext(WithYunwuModelRouting(ctx, disabled, "claude-3-7-sonnet")); got != nil {
		t.Fatalf("expected nil routing when disabled, got %v", got)
	}

	// Enabled routing resolves by raw model name.
	group := &Group{
		ModelRoutingEnabled: true,
		ModelRouting: map[string][]int64{
			"claude-3-7-sonnet":       {11},
			"anthropic/claude-opus-5": {21, 22},
		},
	}
	got := yunwu.ModelRoutingFromContext(WithYunwuModelRouting(ctx, group, "claude-3-7-sonnet"))
	if len(got) != 1 || got[0] != 11 {
		t.Fatalf("expected [11], got %v", got)
	}

	// Resolves by canonical id.
	got = yunwu.ModelRoutingFromContext(WithYunwuModelRouting(ctx, group, "anthropic/claude-opus-5"))
	if len(got) != 2 || got[0] != 21 || got[1] != 22 {
		t.Fatalf("expected [21 22], got %v", got)
	}

	// Unknown model → no routing.
	if got := yunwu.ModelRoutingFromContext(WithYunwuModelRouting(ctx, group, "unknown-model")); got != nil {
		t.Fatalf("expected nil routing for unknown model, got %v", got)
	}

	// Empty model → no routing.
	if got := yunwu.ModelRoutingFromContext(WithYunwuModelRouting(ctx, group, "")); got != nil {
		t.Fatalf("expected nil routing for empty model, got %v", got)
	}
}

func TestContextWithYunwuMaxPricing(t *testing.T) {
	if YunwuMaxPricingFromContext(context.Background()) {
		t.Fatal("expected false for plain context")
	}
	if !YunwuMaxPricingFromContext(ContextWithYunwuMaxPricing(context.Background())) {
		t.Fatal("expected true after ContextWithYunwuMaxPricing")
	}
}
