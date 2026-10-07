package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseModelMarkups(t *testing.T) {
	assert.Nil(t, parseModelMarkups(""))
	assert.Nil(t, parseModelMarkups("{}"))
	assert.Nil(t, parseModelMarkups("not-json"))

	got := parseModelMarkups(`{"Claude-Opus-4": 1.5, "gpt-5*": 1.2, "": 2.0, "bad": 0, "loss": 0.8}`)
	// Keys are lowercased/trimmed; empty keys and multipliers below 1.0 dropped.
	assert.Equal(t, map[string]float64{"claude-opus-4": 1.5, "gpt-5*": 1.2}, got)
}

func TestMatchModelMarkupPattern(t *testing.T) {
	assert.True(t, matchModelMarkupPattern("claude-opus-*", "claude-opus-4"))
	assert.True(t, matchModelMarkupPattern("*sonnet*", "claude-3-7-sonnet"))
	assert.True(t, matchModelMarkupPattern("*", "anything"))
	assert.True(t, matchModelMarkupPattern("gpt-4o", "gpt-4o"))
	assert.False(t, matchModelMarkupPattern("gpt-4o", "gpt-4o-mini"))
	assert.False(t, matchModelMarkupPattern("claude-opus-*", "gpt-4o"))
	assert.False(t, matchModelMarkupPattern("", "x"))
	assert.False(t, matchModelMarkupPattern("x", ""))
}
