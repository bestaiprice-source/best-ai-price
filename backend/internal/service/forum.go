package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	ForumTopicStatusActive = domain.ForumTopicStatusActive
	ForumTopicStatusHidden = domain.ForumTopicStatusHidden

	ForumCommentStatusActive = domain.ForumCommentStatusActive
	ForumCommentStatusHidden = domain.ForumCommentStatusHidden
)

const (
	ForumVoteTypeLike    = domain.ForumVoteTypeLike
	ForumVoteTypeDislike = domain.ForumVoteTypeDislike
	ForumVoteTypeNone    = domain.ForumVoteTypeNone
)

var (
	ErrForumTopicNotFound    = domain.ErrForumTopicNotFound
	ErrForumCommentNotFound  = domain.ErrForumCommentNotFound
	ErrForumInvalidVoteType  = domain.ErrForumInvalidVoteType
	ErrForumTitleEmpty       = domain.ErrForumTitleEmpty
	ErrForumTitleTooLong     = domain.ErrForumTitleTooLong
	ErrForumContentEmpty     = domain.ErrForumContentEmpty
	ErrForumContentTooLong   = domain.ErrForumContentTooLong
	ErrForumParentNotFound   = domain.ErrForumParentNotFound
	ErrCannotDeleteForumPost = domain.ErrCannotDeleteForumPost
)

type ForumTopic = domain.ForumTopic
type CreateForumTopicInput = domain.CreateForumTopicInput
type ForumTopicListQuery = domain.ForumTopicListQuery
type VoteForumTopicInput = domain.VoteForumTopicInput

type ForumComment = domain.ForumComment
type CreateForumCommentInput = domain.CreateForumCommentInput
type ForumCommentListQuery = domain.ForumCommentListQuery
type VoteForumCommentInput = domain.VoteForumCommentInput

type ForumVoteResult = domain.ForumVoteResult

type ForumRepository interface {
	// Topics
	CreateTopic(ctx context.Context, topic *ForumTopic) error
	GetTopicByID(ctx context.Context, id int64) (*ForumTopic, error)
	ListTopics(ctx context.Context, query ForumTopicListQuery) ([]ForumTopic, *pagination.PaginationResult, error)
	SetTopicVote(ctx context.Context, topicID, userID int64, voteType string) (*ForumVoteResult, error)
	GetTopicUserVotes(ctx context.Context, userID int64, topicIDs []int64) (map[int64]string, error)
	AdminListTopics(ctx context.Context, status string, page, pageSize int) ([]ForumTopic, *pagination.PaginationResult, error)
	AdminSetTopicStatus(ctx context.Context, id int64, status string) error

	// Comments
	CreateComment(ctx context.Context, comment *ForumComment) error
	GetCommentByID(ctx context.Context, id int64) (*ForumComment, error)
	ListCommentsByTopic(ctx context.Context, query ForumCommentListQuery) ([]ForumComment, *pagination.PaginationResult, error)
	ListCommentReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]ForumComment, *pagination.PaginationResult, error)
	IncrementTopicCommentCount(ctx context.Context, topicID int64) error
	IncrementCommentReplyCount(ctx context.Context, commentID int64) error
	SetCommentVote(ctx context.Context, commentID, userID int64, voteType string) (*ForumVoteResult, error)
	GetCommentUserVotes(ctx context.Context, userID int64, commentIDs []int64) (map[int64]string, error)
	AdminListComments(ctx context.Context, status string, page, pageSize int) ([]ForumComment, *pagination.PaginationResult, error)
	AdminSetCommentStatus(ctx context.Context, id int64, status string) error
}
