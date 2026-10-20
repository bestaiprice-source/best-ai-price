//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// userErrRepoFake records the filter it was called with and returns one row.
type userErrRepoFake struct {
	opsRepoMock
	gotFilter *OpsErrorLogFilter
}

func (f *userErrRepoFake) ListErrorLogs(_ context.Context, filter *OpsErrorLogFilter) (*OpsErrorLogList, error) {
	f.gotFilter = filter
	return &OpsErrorLogList{
		Errors: []*OpsErrorLog{{
			ID:              1,
			CreatedAt:       time.Now(),
			RequestID:       "req-1",
			Model:           "claude-sonnet-5",
			Platform:        "yunwu",
			InboundEndpoint: "/v1/messages",
			StatusCode:      429,
			Phase:           "upstream",
			Type:            "rate_limit",
			Message:         "rate limit exceeded",
			IsRetryable:     true,
			RetryCount:      2,
			// Internal fields that must NOT be exposed by the user DTO:
			UserEmail:   "victim@example.com",
			AccountID:   ptrInt64(42),
			AccountName: "internal-upstream",
			GroupName:   "internal-group",
		}},
		Total:    1,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

// The user error log must always be scoped to the caller and must reject any
// attempt to widen it (other users, accounts, groups).
func TestGetUserErrorLogs_EnforcesUserScope(t *testing.T) {
	repo := &userErrRepoFake{}
	svc := &OpsService{opsRepo: repo}

	// A hostile filter: asks for another user's account/group and a user search.
	wide := &OpsErrorLogFilter{
		AccountID: ptrInt64(999),
		GroupID:   ptrInt64(999),
		UserQuery: "someone-else@example.com",
		View:      "excluded",
	}
	res, err := svc.GetUserErrorLogs(context.Background(), 7, wide)
	require.NoError(t, err)

	require.NotNil(t, repo.gotFilter)
	require.NotNil(t, repo.gotFilter.UserID)
	assert.EqualValues(t, 7, *repo.gotFilter.UserID, "the caller's id must be forced")
	assert.Nil(t, repo.gotFilter.AccountID, "account scope must be cleared")
	assert.Nil(t, repo.gotFilter.GroupID, "group scope must be cleared")
	assert.Empty(t, repo.gotFilter.UserQuery, "user search must be cleared")
	assert.Equal(t, "all", repo.gotFilter.View, "users must also see business-limit failures")

	require.Len(t, res.Items, 1)
	item := res.Items[0]
	assert.Equal(t, int64(1), item.ID)
	assert.Equal(t, "req-1", item.RequestID)
	assert.Equal(t, 429, item.StatusCode)
	assert.Equal(t, "rate_limit", item.ErrorType)
	assert.Equal(t, "rate limit exceeded", item.Message)
	assert.True(t, item.IsRetryable)
	assert.Equal(t, 2, item.RetryCount)
	assert.Equal(t, "/v1/messages", item.Endpoint)
	// Total is propagated for pagination.
	assert.Equal(t, 1, res.Total)
}

// Degenerate inputs must be safe no-ops (never leak another user's data).
func TestGetUserErrorLogs_Guards(t *testing.T) {
	repo := &userErrRepoFake{}
	svc := &OpsService{opsRepo: repo}

	res, err := svc.GetUserErrorLogs(context.Background(), 0, nil)
	require.NoError(t, err)
	assert.Empty(t, res.Items)
	assert.Nil(t, repo.gotFilter, "no query must be issued without a user")

	// Pagination defaults are applied and clamped.
	_, err = svc.GetUserErrorLogs(context.Background(), 5, &OpsErrorLogFilter{PageSize: 5000})
	require.NoError(t, err)
	require.NotNil(t, repo.gotFilter)
	assert.Equal(t, 1, repo.gotFilter.Page)
	assert.Equal(t, 100, repo.gotFilter.PageSize)

	// A nil repository (monitoring unavailable) must not panic.
	empty := &OpsService{}
	res, err = empty.GetUserErrorLogs(context.Background(), 5, nil)
	require.NoError(t, err)
	assert.Empty(t, res.Items)
}
