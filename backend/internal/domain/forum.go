package domain

import (
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	ForumTopicStatusActive = "active"
	ForumTopicStatusHidden = "hidden"

	ForumCommentStatusActive = "active"
	ForumCommentStatusHidden = "hidden"
)

const (
	ForumVoteTypeLike    = "like"
	ForumVoteTypeDislike = "dislike"
	ForumVoteTypeNone    = "none"
)

var (
	ErrForumTopicNotFound    = infraerrors.NotFound("FORUM_TOPIC_NOT_FOUND", "forum topic not found")
	ErrForumCommentNotFound  = infraerrors.NotFound("FORUM_COMMENT_NOT_FOUND", "forum comment not found")
	ErrForumInvalidVoteType  = infraerrors.BadRequest("INVALID_VOTE_TYPE", "vote_type must be 'like', 'dislike', or 'none'")
	ErrForumTitleEmpty       = infraerrors.BadRequest("FORUM_TITLE_EMPTY", "topic title cannot be empty")
	ErrForumTitleTooLong     = infraerrors.BadRequest("FORUM_TITLE_TOO_LONG", "topic title exceeds maximum length of 255 characters")
	ErrForumContentEmpty     = infraerrors.BadRequest("FORUM_CONTENT_EMPTY", "content cannot be empty")
	ErrForumContentTooLong   = infraerrors.BadRequest("FORUM_CONTENT_TOO_LONG", "content exceeds maximum length limit")
	ErrForumParentNotFound   = infraerrors.NotFound("PARENT_COMMENT_NOT_FOUND", "parent comment not found")
	ErrCannotDeleteForumPost = infraerrors.Forbidden("CANNOT_DELETE_FORUM_POST", "forum topics and comments cannot be deleted; only likes and dislikes can be removed")
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

type ForumTopic struct {
	ID              int64     `json:"id"`
	UserID          int64     `json:"user_id"`
	UserEmailMasked string    `json:"user_email_masked,omitempty"`
	Title           string    `json:"title"`
	Content         string    `json:"content"`
	Status          string    `json:"status"`
	LikeCount       int       `json:"like_count"`
	DislikeCount    int       `json:"dislike_count"`
	CommentCount    int       `json:"comment_count"`
	UserVote        string    `json:"user_vote,omitempty"` // "like", "dislike", or empty/null
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type CreateForumTopicInput struct {
	UserID  int64  `json:"user_id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type ForumTopicListQuery struct {
	Search   string // Optional keyword search by title
	Sort     string // "latest", "top_liked", "most_comments"
	Page     int
	PageSize int
	UserID   int64 // Calling user ID (to populate UserVote)
}

type VoteForumTopicInput struct {
	TopicID  int64  `json:"topic_id"`
	UserID   int64  `json:"user_id"`
	VoteType string `json:"vote_type"` // "like", "dislike", "none"
}

type ForumComment struct {
	ID              int64     `json:"id"`
	TopicID         int64     `json:"topic_id"`
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

type CreateForumCommentInput struct {
	TopicID  int64  `json:"topic_id"`
	UserID   int64  `json:"user_id"`
	ParentID *int64 `json:"parent_id,omitempty"`
	Content  string `json:"content"`
}

type ForumCommentListQuery struct {
	TopicID  int64
	Sort     string // "latest", "oldest", "top_liked"
	Page     int
	PageSize int
	UserID   int64 // Calling user ID (to populate UserVote)
}

type VoteForumCommentInput struct {
	CommentID int64  `json:"comment_id"`
	UserID    int64  `json:"user_id"`
	VoteType  string `json:"vote_type"` // "like", "dislike", "none"
}

type ForumVoteResult struct {
	ID           int64  `json:"id"`
	LikeCount    int    `json:"like_count"`
	DislikeCount int    `json:"dislike_count"`
	UserVote     string `json:"user_vote"` // "like", "dislike", or "none"
}
