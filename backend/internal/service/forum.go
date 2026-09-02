package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	ForumPostStatusActive = domain.ForumPostStatusActive
	ForumPostStatusHidden = domain.ForumPostStatusHidden
)

const (
	ForumVoteTypeLike    = domain.ForumVoteTypeLike
	ForumVoteTypeDislike = domain.ForumVoteTypeDislike
	ForumVoteTypeNone    = domain.ForumVoteTypeNone
)

var (
	ErrForumPostNotFound     = domain.ErrForumPostNotFound
	ErrForumInvalidVoteType  = domain.ErrForumInvalidVoteType
	ErrForumContentEmpty     = domain.ErrForumContentEmpty
	ErrForumContentTooLong   = domain.ErrForumContentTooLong
	ErrForumParentNotFound   = domain.ErrForumParentNotFound
	ErrCannotDeleteForumPost = domain.ErrCannotDeleteForumPost
)

type ForumPost = domain.ForumPost
type CreateForumPostInput = domain.CreateForumPostInput
type VoteForumPostInput = domain.VoteForumPostInput
type ForumVoteResult = domain.ForumVoteResult
type ForumPostListQuery = domain.ForumPostListQuery

type ForumRepository interface {
	CreatePost(ctx context.Context, post *ForumPost) error
	GetByID(ctx context.Context, id int64) (*ForumPost, error)
	ListPosts(ctx context.Context, query ForumPostListQuery) ([]ForumPost, *pagination.PaginationResult, error)
	ListReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]ForumPost, *pagination.PaginationResult, error)
	IncrementReplyCount(ctx context.Context, postID int64) error
	SetVote(ctx context.Context, postID, userID int64, voteType string) (*ForumVoteResult, error)
	GetUserVotes(ctx context.Context, userID int64, postIDs []int64) (map[int64]string, error)
	AdminListPosts(ctx context.Context, status string, page, pageSize int) ([]ForumPost, *pagination.PaginationResult, error)
	AdminSetStatus(ctx context.Context, id int64, status string) error
}
