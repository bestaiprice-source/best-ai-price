//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type accountRepoStubForGroupValidation struct {
	accountRepoStub
	createdAccount *Account
	updatedAccount *Account
	boundGroups    map[int64][]int64
	accountToGet   *Account
}

func (s *accountRepoStubForGroupValidation) Create(ctx context.Context, account *Account) error {
	account.ID = 999
	s.createdAccount = account
	return nil
}

func (s *accountRepoStubForGroupValidation) GetByID(ctx context.Context, id int64) (*Account, error) {
	if s.accountToGet != nil {
		return s.accountToGet, nil
	}
	return nil, ErrAccountNotFound
}

func (s *accountRepoStubForGroupValidation) Update(ctx context.Context, account *Account) error {
	s.updatedAccount = account
	return nil
}

func (s *accountRepoStubForGroupValidation) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	if s.boundGroups == nil {
		s.boundGroups = make(map[int64][]int64)
	}
	s.boundGroups[accountID] = groupIDs
	return nil
}

type groupRepoStubForAccountValidation struct{}

func (g *groupRepoStubForAccountValidation) ExistsByIDs(ctx context.Context, ids []int64) (map[int64]bool, error) {
	res := make(map[int64]bool, len(ids))
	for _, id := range ids {
		res[id] = true
	}
	return res, nil
}

func (g *groupRepoStubForAccountValidation) Create(ctx context.Context, group *Group) error {
	return nil
}

func (g *groupRepoStubForAccountValidation) GetByID(ctx context.Context, id int64) (*Group, error) {
	return nil, nil
}

func (g *groupRepoStubForAccountValidation) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	return nil, nil
}

func (g *groupRepoStubForAccountValidation) List(ctx context.Context, params pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (g *groupRepoStubForAccountValidation) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (g *groupRepoStubForAccountValidation) Update(ctx context.Context, group *Group) error {
	return nil
}

func (g *groupRepoStubForAccountValidation) Delete(ctx context.Context, id int64) error {
	return nil
}

func (g *groupRepoStubForAccountValidation) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	return nil, nil
}

func (g *groupRepoStubForAccountValidation) ListActive(ctx context.Context) ([]Group, error) {
	return nil, nil
}

func (g *groupRepoStubForAccountValidation) ListActiveByPlatform(ctx context.Context, platform string) ([]Group, error) {
	return nil, nil
}

func (g *groupRepoStubForAccountValidation) ExistsByName(ctx context.Context, name string) (bool, error) {
	return false, nil
}

func (g *groupRepoStubForAccountValidation) GetAccountCount(ctx context.Context, groupID int64) (total int64, active int64, err error) {
	return 0, 0, nil
}

func (g *groupRepoStubForAccountValidation) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
}

func (g *groupRepoStubForAccountValidation) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	return nil, nil
}

func (g *groupRepoStubForAccountValidation) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	return nil
}

func (g *groupRepoStubForAccountValidation) UpdateSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error {
	return nil
}

func TestYunwuAccountGroupRequirement(t *testing.T) {
	ctx := context.Background()

	t.Run("Create yunwu account without groups fails", func(t *testing.T) {
		accRepo := &accountRepoStubForGroupValidation{}
		grpRepo := &groupRepoStubForAccountValidation{}
		svc := NewAccountService(accRepo, grpRepo)

		req := CreateAccountRequest{
			Name:     "yunwu-test-no-groups",
			Platform: "yunwu",
			GroupIDs: nil,
		}
		_, err := svc.Create(ctx, req)
		require.Error(t, err)
		assert.Equal(t, ErrYunwuAccountGroupRequired, err)
	})

	t.Run("Create yunwu account with empty groups slice fails", func(t *testing.T) {
		accRepo := &accountRepoStubForGroupValidation{}
		grpRepo := &groupRepoStubForAccountValidation{}
		svc := NewAccountService(accRepo, grpRepo)

		req := CreateAccountRequest{
			Name:     "yunwu-test-empty-groups",
			Platform: "yunwu",
			GroupIDs: []int64{},
		}
		_, err := svc.Create(ctx, req)
		require.Error(t, err)
		assert.Equal(t, ErrYunwuAccountGroupRequired, err)
	})

	t.Run("Create yunwu account with group succeeds", func(t *testing.T) {
		accRepo := &accountRepoStubForGroupValidation{}
		grpRepo := &groupRepoStubForAccountValidation{}
		svc := NewAccountService(accRepo, grpRepo)

		req := CreateAccountRequest{
			Name:     "yunwu-test-with-groups",
			Platform: "yunwu",
			GroupIDs: []int64{101},
		}
		acc, err := svc.Create(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, acc)
		assert.Equal(t, []int64{101}, accRepo.boundGroups[999])
	})

	t.Run("Create non-yunwu account without groups succeeds", func(t *testing.T) {
		accRepo := &accountRepoStubForGroupValidation{}
		grpRepo := &groupRepoStubForAccountValidation{}
		svc := NewAccountService(accRepo, grpRepo)

		req := CreateAccountRequest{
			Name:     "openai-test-no-groups",
			Platform: "openai",
			GroupIDs: nil,
		}
		acc, err := svc.Create(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, acc)
	})

	t.Run("Update yunwu account to empty groups fails", func(t *testing.T) {
		accRepo := &accountRepoStubForGroupValidation{
			accountToGet: &Account{
				ID:       123,
				Platform: "yunwu",
				Name:     "existing-yunwu",
			},
		}
		grpRepo := &groupRepoStubForAccountValidation{}
		svc := NewAccountService(accRepo, grpRepo)

		emptyGroups := []int64{}
		req := UpdateAccountRequest{
			GroupIDs: &emptyGroups,
		}
		_, err := svc.Update(ctx, 123, req)
		require.Error(t, err)
		assert.Equal(t, ErrYunwuAccountGroupRequired, err)
	})

	t.Run("Update yunwu account with non-empty groups succeeds", func(t *testing.T) {
		accRepo := &accountRepoStubForGroupValidation{
			accountToGet: &Account{
				ID:       123,
				Platform: "yunwu",
				Name:     "existing-yunwu",
			},
		}
		grpRepo := &groupRepoStubForAccountValidation{}
		svc := NewAccountService(accRepo, grpRepo)

		newGroups := []int64{202}
		req := UpdateAccountRequest{
			GroupIDs: &newGroups,
		}
		acc, err := svc.Update(ctx, 123, req)
		require.NoError(t, err)
		assert.NotNil(t, acc)
		assert.Equal(t, []int64{202}, accRepo.boundGroups[123])
	})
}
