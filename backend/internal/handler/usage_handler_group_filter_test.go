package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// The group filter is scoped by the caller's own UserID in the same query, so a
// group id can only ever narrow the result set — it must reach the repository
// filter unchanged.
func TestUserUsageListGroupFilter(t *testing.T) {
	repo := &userUsageRepoCapture{}
	router := newUserUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/usage?group_id=7", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(42), repo.listFilters.UserID)
	require.Equal(t, int64(7), repo.listFilters.GroupID)
}

func TestUserUsageListGroupFilterOmitted(t *testing.T) {
	repo := &userUsageRepoCapture{}
	router := newUserUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/usage", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Zero(t, repo.listFilters.GroupID)
}

func TestUserUsageListInvalidGroupID(t *testing.T) {
	repo := &userUsageRepoCapture{}
	router := newUserUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/usage?group_id=not-a-number", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Zero(t, repo.listFilters.GroupID)
}
