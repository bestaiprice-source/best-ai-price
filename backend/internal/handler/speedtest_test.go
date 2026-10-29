package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
)

func TestSpeedTestWriterStampsFirstByteAndDiscardsPayload(t *testing.T) {
	writer := &speedTestWriter{startedAt: time.Now().Add(-10 * time.Millisecond)}
	if _, err := writer.Write([]byte("data: {\"choices\":[]}\n\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	first := writer.firstByte
	if first.IsZero() {
		t.Fatal("first byte was not stamped")
	}
	if got := writer.ttftMs(time.Now()); got < 5 {
		t.Fatalf("expected a positive TTFT, got %d", got)
	}

	time.Sleep(5 * time.Millisecond)
	if _, err := writer.Write([]byte("more")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// The first write stamps TTFT; later writes must not move it, or TTFT would
	// silently become the total duration.
	if !writer.firstByte.Equal(first) {
		t.Fatal("a later write must not move the first-byte stamp")
	}
}

func TestSpeedTestWriterFallsBackToTotalTimeWhenNothingStreamed(t *testing.T) {
	writer := &speedTestWriter{startedAt: time.Now().Add(-25 * time.Millisecond)}
	if got := writer.ttftMs(time.Now()); got < 20 {
		t.Fatalf("a silent stream must report the elapsed time, got %d", got)
	}
}

func TestSpeedTestWriterIgnoresEmptyWrites(t *testing.T) {
	writer := &speedTestWriter{startedAt: time.Now()}
	if _, err := writer.Write(nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !writer.firstByte.IsZero() {
		t.Fatal("an empty write is not a first token")
	}
}

func TestBuildSpeedTestPayloadIsAMinimalRealRequest(t *testing.T) {
	raw, err := buildSpeedTestPayload("anthropic/claude-sonnet-5")
	if err != nil {
		t.Fatalf("buildSpeedTestPayload: %v", err)
	}
	var payload struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Stream    bool   `json:"stream"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if payload.Model != "anthropic/claude-sonnet-5" {
		t.Fatalf("model must be forwarded verbatim, got %q", payload.Model)
	}
	if payload.MaxTokens != speedTestMaxTokens {
		t.Fatalf("expected max_tokens %d, got %d", speedTestMaxTokens, payload.MaxTokens)
	}
	if !payload.Stream {
		t.Fatal("the probe must stream: TTFT is the measurement")
	}
	if len(payload.Messages) != 1 || payload.Messages[0].Content != speedTestPrompt {
		t.Fatalf("unexpected messages: %+v", payload.Messages)
	}
}

func TestSpeedTestErrorStatusClassifiesEngineFailures(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantType   string
	}{
		{"unpriced model", yunwu.ErrNoPricingAvailable, http.StatusBadRequest, "invalid_request_error"},
		{"no compatible nodes", yunwu.ErrNoCompatibleNodes, http.StatusBadRequest, "invalid_request_error"},
		{"tier not configured", yunwu.ErrTierNotConfigured, http.StatusBadRequest, "invalid_request_error"},
		{"upstream failure", errors.New("upstream 502"), http.StatusBadGateway, "api_error"},
		{"nil error", nil, http.StatusBadGateway, "api_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, errType := speedTestErrorStatus(tc.err)
			if status != tc.wantStatus || errType != tc.wantType {
				t.Fatalf("got (%d, %s), want (%d, %s)", status, errType, tc.wantStatus, tc.wantType)
			}
		})
	}
}
