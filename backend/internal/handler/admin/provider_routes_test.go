package admin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMatchModelPattern(t *testing.T) {
	assert.True(t, matchModelPattern("claude-opus-*", "claude-opus-4"))
	assert.True(t, matchModelPattern("claude-opus-*", "claude-opus-4-1"))
	assert.True(t, matchModelPattern("claude-3-7-sonnet", "CLAUDE-3-7-SONNET"))
	assert.True(t, matchModelPattern("*", "anything"))
	assert.True(t, matchModelPattern("*sonnet*", "claude-3-7-sonnet"))
	assert.False(t, matchModelPattern("claude-opus-*", "gpt-4o"))
	assert.False(t, matchModelPattern("claude-3-7-sonnet", "claude-3-5-sonnet"))
	assert.False(t, matchModelPattern("", "claude"))
	assert.False(t, matchModelPattern("claude", ""))
}

func TestRouteStatus(t *testing.T) {
	up := &ModelRouteProvider{Available: true}
	down := &ModelRouteProvider{Available: false}

	assert.Equal(t, "unavailable", routeStatus(nil, up))
	assert.Equal(t, "available", routeStatus(up, up))
	assert.Equal(t, "available", routeStatus(up, nil))
	assert.Equal(t, "degraded", routeStatus(up, down))
	assert.Equal(t, "degraded", routeStatus(down, up))
	assert.Equal(t, "unavailable", routeStatus(down, down))
	assert.Equal(t, "unavailable", routeStatus(down, nil))
}

func TestLastVerified(t *testing.T) {
	older := "2026-10-01T10:00:00Z"
	newer := "2026-10-07T10:00:00Z"

	// Both legs verified -> the older success (when both were last confirmed).
	p := &ModelRouteProvider{LastSuccessAt: &newer}
	b := &ModelRouteProvider{LastSuccessAt: &older}
	res := lastVerified(p, b)
	assert.NotNil(t, res)
	assert.Equal(t, older, *res)

	// Single route -> that route's last success.
	res = lastVerified(p, nil)
	assert.NotNil(t, res)
	assert.Equal(t, newer, *res)

	// A leg never verified -> unknown.
	assert.Nil(t, lastVerified(p, &ModelRouteProvider{}))
	assert.Nil(t, lastVerified(nil, b))

	_ = time.Now
}
