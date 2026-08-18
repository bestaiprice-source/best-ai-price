package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type ProviderCommentService struct {
	repo ProviderCommentRepository
}

func NewProviderCommentService(repo ProviderCommentRepository) *ProviderCommentService {
	return &ProviderCommentService{
		repo: repo,
	}
}

func (s *ProviderCommentService) CreateComment(ctx context.Context, input CreateCommentInput) (*ProviderComment, error) {
	provider := strings.TrimSpace(input.Provider)
	content := strings.TrimSpace(input.Content)

	if content == "" {
		return nil, ErrCommentContentEmpty
	}
	if len([]rune(content)) > 5000 {
		return nil, ErrCommentContentTooLong
	}

	sentiment := domain.NormalizeSentiment(input.Sentiment)
	var parentComment *ProviderComment

	if input.ParentID != nil && *input.ParentID > 0 {
		parent, err := s.repo.GetByID(ctx, *input.ParentID)
		if err != nil {
			return nil, ErrParentCommentNotFound
		}
		parentComment = parent
		provider = parent.Provider
		sentiment = SentimentNeutral
	} else {
		if provider == "" {
			provider = "default"
		}
		if sentiment != SentimentPositive && sentiment != SentimentNegative {
			sentiment = SentimentPositive
		}
	}

	comment := &ProviderComment{
		Provider:     provider,
		UserID:       input.UserID,
		ParentID:     input.ParentID,
		Content:      content,
		Sentiment:    sentiment,
		Status:       CommentStatusActive,
		LikeCount:    0,
		DislikeCount: 0,
		ReplyCount:   0,
	}

	if err := s.repo.CreateComment(ctx, comment); err != nil {
		return nil, err
	}

	if parentComment != nil {
		_ = s.repo.IncrementReplyCount(ctx, parentComment.ID)
	}

	return comment, nil
}

func (s *ProviderCommentService) ListComments(ctx context.Context, query CommentListQuery) ([]ProviderComment, *pagination.PaginationResult, *ProviderCommentSummary, error) {
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}
	query.Provider = strings.TrimSpace(query.Provider)

	comments, pagResult, err := s.repo.ListProviderComments(ctx, query)
	if err != nil {
		return nil, nil, nil, err
	}

	summary, err := s.repo.GetProviderSummary(ctx, query.Provider)
	if err != nil {
		summary = &ProviderCommentSummary{
			Provider: query.Provider,
		}
	}

	return comments, pagResult, summary, nil
}

func (s *ProviderCommentService) ListReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]ProviderComment, *pagination.PaginationResult, error) {
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
		return nil, nil, ErrCommentNotFound
	}

	return s.repo.ListReplies(ctx, parentID, page, pageSize, currentUserID)
}

func (s *ProviderCommentService) VoteComment(ctx context.Context, input VoteCommentInput) (*VoteResult, error) {
	voteType := domain.NormalizeVoteType(input.VoteType)
	if voteType == "" {
		return nil, ErrInvalidVoteType
	}

	// Verify target comment exists and is active
	comment, err := s.repo.GetByID(ctx, input.CommentID)
	if err != nil {
		return nil, ErrCommentNotFound
	}
	if comment.Status != CommentStatusActive {
		return nil, ErrCommentNotFound
	}

	return s.repo.SetVote(ctx, input.CommentID, input.UserID, voteType)
}

func (s *ProviderCommentService) GetProviderSummary(ctx context.Context, provider string) (*ProviderCommentSummary, error) {
	return s.repo.GetProviderSummary(ctx, strings.TrimSpace(provider))
}

func (s *ProviderCommentService) AdminListComments(ctx context.Context, provider, status, sentiment string, page, pageSize int) ([]ProviderComment, *pagination.PaginationResult, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return s.repo.AdminListComments(ctx, strings.TrimSpace(provider), strings.TrimSpace(status), strings.TrimSpace(sentiment), page, pageSize)
}

func (s *ProviderCommentService) AdminSetStatus(ctx context.Context, id int64, status string) error {
	if status != CommentStatusActive && status != CommentStatusHidden {
		status = CommentStatusHidden
	}
	return s.repo.AdminSetStatus(ctx, id, status)
}
