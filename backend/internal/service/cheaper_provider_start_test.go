package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheaperProviderStartDisabledIsNoOp(t *testing.T) {
	// interval <= 0 disables the periodic scan: Start must return without
	// spawning a ticker goroutine, and repeated calls must be safe (sync.Once).
	t.Setenv("ASH_CHEAPER_PROVIDER_CHECK_SECONDS", "0")
	svc := &CheaperProviderService{}
	assert.NotPanics(t, func() {
		svc.Start()
		svc.Start()
	})
}
