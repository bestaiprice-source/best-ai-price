package yunwu

import (
	"errors"
	"net/http"
	"testing"
)

func TestClassifyProbeStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		err    error
		want   probeOutcome
	}{
		{"200 healthy", http.StatusOK, nil, probeHealthy},
		{"204 healthy", http.StatusNoContent, nil, probeHealthy},
		{"404 reachable", http.StatusNotFound, nil, probeHealthy},
		{"405 reachable", http.StatusMethodNotAllowed, nil, probeHealthy},
		{"400 reachable", http.StatusBadRequest, nil, probeHealthy},
		{"401 invalid key", http.StatusUnauthorized, nil, probeAuthInvalid},
		{"403 invalid key", http.StatusForbidden, nil, probeAuthInvalid},
		{"429 rate limited", http.StatusTooManyRequests, nil, probeRateLimited},
		{"500 server error", http.StatusInternalServerError, nil, probeServerError},
		{"503 server error", http.StatusServiceUnavailable, nil, probeServerError},
		{"network error", 0, errors.New("connection refused"), probeUnreachable},
	}
	for _, tc := range cases {
		if got := classifyProbeStatus(tc.status, tc.err); got != tc.want {
			t.Errorf("%s: classifyProbeStatus(%d,%v) = %d, want %d", tc.name, tc.status, tc.err, got, tc.want)
		}
	}
}
