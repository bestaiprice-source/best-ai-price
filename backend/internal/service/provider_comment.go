package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	SentimentPositive = domain.SentimentPositive
	SentimentNegative = domain.SentimentNegative
	SentimentNeutral  = domain.SentimentNeutral
)

const (
	CommentStatusActive = domain.CommentStatusActive
	CommentStatusHidden = domain.CommentStatusHidden
)

const (
	VoteTypeLike    = domain.VoteTypeLike
	VoteTypeDislike = domain.VoteTypeDislike
	VoteTypeNone    = domain.VoteTypeNone
)

var (
	ErrCommentNotFound       = domain.ErrCommentNotFound
	ErrInvalidSentiment      = domain.ErrInvalidSentiment
	ErrInvalidVoteType       = domain.ErrInvalidVoteType
	ErrCommentContentEmpty   = domain.ErrCommentContentEmpty
	ErrCommentContentTooLong = domain.ErrCommentContentTooLong
	ErrParentCommentNotFound = domain.ErrParentCommentNotFound
	ErrCannotDeleteComment   = domain.ErrCannotDeleteComment
)

type ProviderComment = domain.ProviderComment
type ProviderCommentSummary = domain.ProviderCommentSummary
type CreateCommentInput = domain.CreateCommentInput
type VoteCommentInput = domain.VoteCommentInput
type VoteResult = domain.VoteResult
type CommentListQuery = domain.CommentListQuery

type ProviderCommentRepository interface {
	CreateComment(ctx context.Context, c *ProviderComment) error
	GetByID(ctx context.Context, id int64) (*ProviderComment, error)
	ListProviderComments(ctx context.Context, query CommentListQuery) ([]ProviderComment, *pagination.PaginationResult, error)
	ListReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]ProviderComment, *pagination.PaginationResult, error)
	GetProviderSummary(ctx context.Context, provider string) (*ProviderCommentSummary, error)
	IncrementReplyCount(ctx context.Context, commentID int64) error
	SetVote(ctx context.Context, commentID, userID int64, voteType string) (*VoteResult, error)
	GetUserVotes(ctx context.Context, userID int64, commentIDs []int64) (map[int64]string, error)
	AdminListComments(ctx context.Context, provider, status, sentiment string, page, pageSize int) ([]ProviderComment, *pagination.PaginationResult, error)
	AdminSetStatus(ctx context.Context, id int64, status string) error
}
