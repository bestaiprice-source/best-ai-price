package domain

import (
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	ForumPostStatusActive = "active"
	ForumPostStatusHidden = "hidden"
)

const (
	ForumVoteTypeLike    = "like"
	ForumVoteTypeDislike = "dislike"
	ForumVoteTypeNone    = "none"
)

var (
	ErrForumPostNotFound   = infraerrors.NotFound("FORUM_POST_NOT_FOUND", "forum post not found")
	ErrForumInvalidVoteType = infraerrors.BadRequest("INVALID_VOTE_TYPE", "vote_type must be 'like', 'dislike', or 'none'")
	ErrForumContentEmpty   = infraerrors.BadRequest("FORUM_CONTENT_EMPTY", "post content cannot be empty")
	ErrForumContentTooLong = infraerrors.BadRequest("FORUM_CONTENT_TOO_LONG", "post content exceeds maximum length of 5000 characters")
	ErrForumParentNotFound = infraerrors.NotFound("PARENT_POST_NOT_FOUND", "parent post not found")
	ErrCannotDeleteForumPost = infraerrors.Forbidden("CANNOT_DELETE_FORUM_POST", "forum posts cannot be deleted; only likes and dislikes can be removed")
)

func NormalizeForumVoteType(v string) string {
	lower := strings.ToLower(strings.TrimSpace(v))
	switch lower {
	case ForumVoteTypeLike:
		return ForumVoteTypeLike
	case ForumVoteTypeDislike:
		return ForumVoteTypeDislike
	case ForumVoteTypeNone, "":
		return ForumVoteTypeNone
	default:
		return ""
	}
}

type ForumPost struct {
	ID              int64     `json:"id"`
	UserID          int64     `json:"user_id"`
	UserEmailMasked string    `json:"user_email_masked,omitempty"`
	ParentID        *int64    `json:"parent_id,omitempty"`
	Content         string    `json:"content"`
	Status          string    `json:"status"`
	LikeCount       int       `json:"like_count"`
	DislikeCount    int       `json:"dislike_count"`
	ReplyCount      int       `json:"reply_count"`
	UserVote        string    `json:"user_vote,omitempty"` // "like", "dislike", or empty/null
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type CreateForumPostInput struct {
	UserID   int64  `json:"user_id"`
	ParentID *int64 `json:"parent_id,omitempty"`
	Content  string `json:"content"`
}

type VoteForumPostInput struct {
	PostID   int64  `json:"post_id"`
	UserID   int64  `json:"user_id"`
	VoteType string `json:"vote_type"` // "like", "dislike", "none"
}

type ForumVoteResult struct {
	PostID       int64  `json:"post_id"`
	LikeCount    int    `json:"like_count"`
	DislikeCount int    `json:"dislike_count"`
	UserVote     string `json:"user_vote"` // "like", "dislike", or "none"
}

type ForumPostListQuery struct {
	ParentID *int64
	Sort     string // "latest", "top_liked", "most_replies"
	Page     int
	PageSize int
	UserID   int64 // Calling user ID (to populate UserVote)
}
