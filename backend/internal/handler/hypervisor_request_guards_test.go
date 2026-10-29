package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// The Yunwu/hypervisor dispatch paths branch out of the gateway handlers before
// the shared validation block, so these guards are the only enforcement of the
// model allowlist and the per-window spend limits for unofficial-provider keys.
func TestCheckHypervisorRequestGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("model outside the allowlist is rejected with 400", func(t *testing.T) {
		apiKey := &service.APIKey{ID: 1, AllowedModels: []string{"claude-sonnet-5"}}
		err := checkHypervisorRequestGuards(ctx, nil, apiKey, "gpt-4o")
		if err == nil {
			t.Fatal("expected the allowlist to reject gpt-4o")
		}
		var notAllowed *modelNotAllowedError
		if !errors.As(err, &notAllowed) {
			t.Fatalf("expected a modelNotAllowedError, got %T (%v)", err, err)
		}
		status, code, message := hypervisorGuardStatus(err)
		if status != http.StatusBadRequest || code != "invalid_request_error" {
			t.Fatalf("unexpected mapping: %d %s", status, code)
		}
		if message == "" {
			t.Fatal("expected a non-empty message")
		}
	})

	t.Run("allowed model passes", func(t *testing.T) {
		apiKey := &service.APIKey{ID: 1, AllowedModels: []string{"anthropic/claude-sonnet-5"}}
		if err := checkHypervisorRequestGuards(ctx, nil, apiKey, "claude-sonnet-5"); err != nil {
			t.Fatalf("vendor-prefixed allowlist entry must allow the bare model, got %v", err)
		}
	})

	t.Run("wildcard allowlist passes", func(t *testing.T) {
		apiKey := &service.APIKey{ID: 1, AllowedModels: []string{"*"}}
		if err := checkHypervisorRequestGuards(ctx, nil, apiKey, "gpt-4o"); err != nil {
			t.Fatalf("wildcard must allow every model, got %v", err)
		}
	})

	t.Run("empty allowlist passes and no rate windows is a no-op", func(t *testing.T) {
		if err := checkHypervisorRequestGuards(ctx, nil, &service.APIKey{ID: 1}, "gpt-4o"); err != nil {
			t.Fatalf("unrestricted key must pass, got %v", err)
		}
	})

	t.Run("nil key passes", func(t *testing.T) {
		if err := checkHypervisorRequestGuards(ctx, nil, nil, "gpt-4o"); err != nil {
			t.Fatalf("nil key must not error, got %v", err)
		}
	})
}
