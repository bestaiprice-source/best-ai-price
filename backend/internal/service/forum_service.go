package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type ForumService struct {
	repo ForumRepository
}

func NewForumService(repo ForumRepository) *ForumService {
	return &ForumService{
		repo: repo,
	}
}

func (s *ForumService) CreatePost(ctx context.Context, input CreateForumPostInput) (*ForumPost, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, ErrForumContentEmpty
	}
	if len([]rune(content)) > 5000 {
		return nil, ErrForumContentTooLong
	}

	var parentPost *ForumPost
	if input.ParentID != nil && *input.ParentID > 0 {
		parent, err := s.repo.GetByID(ctx, *input.ParentID)
		if err != nil {
			return nil, ErrForumParentNotFound
		}
		parentPost = parent
	}

	post := &ForumPost{
		UserID:       input.UserID,
		ParentID:     input.ParentID,
		Content:      content,
		Status:       ForumPostStatusActive,
		LikeCount:    0,
		DislikeCount: 0,
		ReplyCount:   0,
	}

	if err := s.repo.CreatePost(ctx, post); err != nil {
		return nil, err
	}

	if parentPost != nil {
		_ = s.repo.IncrementReplyCount(ctx, parentPost.ID)
	}

	return post, nil
}

func (s *ForumService) ListPosts(ctx context.Context, query ForumPostListQuery) ([]ForumPost, *pagination.PaginationResult, error) {
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	return s.repo.ListPosts(ctx, query)
}

func (s *ForumService) ListReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]ForumPost, *pagination.PaginationResult, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// Verify parent exists
	if _, err := s.repo.GetByID(ctx, parentID); err != nil {
		return nil, nil, ErrForumPostNotFound
	}

	return s.repo.ListReplies(ctx, parentID, page, pageSize, currentUserID)
}

func (s *ForumService) VotePost(ctx context.Context, input VoteForumPostInput) (*ForumVoteResult, error) {
	voteType := domain.NormalizeForumVoteType(input.VoteType)
	if voteType == "" {
		return nil, ErrForumInvalidVoteType
	}

	// Verify target post exists and is active
	post, err := s.repo.GetByID(ctx, input.PostID)
	if err != nil {
		return nil, ErrForumPostNotFound
	}
	if post.Status != ForumPostStatusActive {
		return nil, ErrForumPostNotFound
	}

	return s.repo.SetVote(ctx, input.PostID, input.UserID, voteType)
}

func (s *ForumService) AdminListPosts(ctx context.Context, status string, page, pageSize int) ([]ForumPost, *pagination.PaginationResult, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return s.repo.AdminListPosts(ctx, strings.TrimSpace(status), page, pageSize)
}

func (s *ForumService) AdminSetStatus(ctx context.Context, id int64, status string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != ForumPostStatusActive && status != ForumPostStatusHidden {
		return ErrForumPostNotFound
	}
	return s.repo.AdminSetStatus(ctx, id, status)
}
