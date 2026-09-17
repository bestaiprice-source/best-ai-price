package service_test

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestCalculateEffectiveConcurrency(t *testing.T) {
	tests := []struct {
		name                 string
		configured           int
		balance              float64
		hasSub               bool
		expectedConcurrency  int
	}{
		{
			name:                "subscription user keeps configured concurrency regardless of balance",
			configured:          5,
			balance:             0.01,
			hasSub:              true,
			expectedConcurrency: 5,
		},
		{
			name:                "subscription user with 0 configured gets 1",
			configured:          0,
			balance:             10.0,
			hasSub:              true,
			expectedConcurrency: 1,
		},
		{
			name:                "payg user balance below LowBalanceConcurrencyThreshold ($1.00) clamped to 1",
			configured:          5,
			balance:             0.50,
			hasSub:              false,
			expectedConcurrency: 1,
		},
		{
			name:                "payg user balance 0.99 clamped to 1",
			configured:          5,
			balance:             0.99,
			hasSub:              false,
			expectedConcurrency: 1,
		},
		{
			name:                "payg user balance exactly $1.00 allows 2 slots",
			configured:          5,
			balance:             1.00,
			hasSub:              false,
			expectedConcurrency: 2,
		},
		{
			name:                "payg user balance $1.49 allows 2 slots",
			configured:          5,
			balance:             1.49,
			hasSub:              false,
			expectedConcurrency: 2,
		},
		{
			name:                "payg user balance $1.50 allows 3 slots",
			configured:          5,
			balance:             1.50,
			hasSub:              false,
			expectedConcurrency: 3,
		},
		{
			name:                "payg user balance $2.50 allows configured 5 slots",
			configured:          5,
			balance:             2.50,
			hasSub:              false,
			expectedConcurrency: 5,
		},
		{
			name:                "payg user high balance capped at configured concurrency",
			configured:          3,
			balance:             100.0,
			hasSub:              false,
			expectedConcurrency: 3,
		},
		{
			name:                "negative or zero configured concurrency defaults to 1",
			configured:          -1,
			balance:             10.0,
			hasSub:              false,
			expectedConcurrency: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.CalculateEffectiveConcurrency(tt.configured, tt.balance, tt.hasSub)
			assert.Equal(t, tt.expectedConcurrency, result)
		})
	}
}
