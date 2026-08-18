package domain

import (
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SentimentPositive = "positive"
	SentimentNegative = "negative"
	SentimentNeutral  = "neutral"
)

const (
	CommentStatusActive = "active"
	CommentStatusHidden = "hidden"
)

const (
	VoteTypeLike    = "like"
	VoteTypeDislike = "dislike"
	VoteTypeNone    = "none"
)

var (
	ErrCommentNotFound        = infraerrors.NotFound("COMMENT_NOT_FOUND", "comment not found")
	ErrInvalidSentiment       = infraerrors.BadRequest("INVALID_SENTIMENT", "sentiment must be 'positive' or 'negative'")
	ErrInvalidVoteType        = infraerrors.BadRequest("INVALID_VOTE_TYPE", "vote_type must be 'like', 'dislike', or 'none'")
	ErrCommentContentEmpty    = infraerrors.BadRequest("COMMENT_CONTENT_EMPTY", "comment content cannot be empty")
	ErrCommentContentTooLong  = infraerrors.BadRequest("COMMENT_CONTENT_TOO_LONG", "comment content exceeds maximum length of 5000 characters")
	ErrParentCommentNotFound  = infraerrors.NotFound("PARENT_COMMENT_NOT_FOUND", "parent comment not found")
	ErrCannotDeleteComment    = infraerrors.Forbidden("CANNOT_DELETE_COMMENT", "comments cannot be deleted; only likes and dislikes can be removed")
)

func NormalizeSentiment(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	switch lower {
	case SentimentPositive:
		return SentimentPositive
	case SentimentNegative:
		return SentimentNegative
	default:
		return SentimentNeutral
	}
}

func NormalizeVoteType(v string) string {
	lower := strings.ToLower(strings.TrimSpace(v))
	switch lower {
	case VoteTypeLike:
		return VoteTypeLike
	case VoteTypeDislike:
		return VoteTypeDislike
	case VoteTypeNone, "":
		return VoteTypeNone
	default:
		return ""
	}
}

type ProviderComment struct {
	ID              int64     `json:"id"`
	Provider        string    `json:"provider"`
	UserID          int64     `json:"user_id"`
	UserEmailMasked string    `json:"user_email_masked,omitempty"`
	ParentID        *int64    `json:"parent_id,omitempty"`
	Content         string    `json:"content"`
	Sentiment       string    `json:"sentiment"`
	Status          string    `json:"status"`
	LikeCount       int       `json:"like_count"`
	DislikeCount    int       `json:"dislike_count"`
	ReplyCount      int       `json:"reply_count"`
	UserVote        string    `json:"user_vote,omitempty"` // "like", "dislike", or empty/null
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ProviderCommentSummary struct {
	Provider       string  `json:"provider"`
	TotalComments  int     `json:"total_comments"`
	PositiveCount  int     `json:"positive_count"`
	NegativeCount  int     `json:"negative_count"`
	PositiveRatio  float64 `json:"positive_ratio"` // e.g. 85.5 (%)
}

type CreateCommentInput struct {
	Provider  string `json:"provider"`
	UserID    int64  `json:"user_id"`
	ParentID  *int64 `json:"parent_id,omitempty"`
	Content   string `json:"content"`
	Sentiment string `json:"sentiment"`
}

type VoteCommentInput struct {
	CommentID int64  `json:"comment_id"`
	UserID    int64  `json:"user_id"`
	VoteType  string `json:"vote_type"` // "like", "dislike", "none"
}

type VoteResult struct {
	CommentID    int64  `json:"comment_id"`
	LikeCount    int    `json:"like_count"`
	DislikeCount int    `json:"dislike_count"`
	UserVote     string `json:"user_vote"` // "like", "dislike", or "none"
}

type CommentListQuery struct {
	Provider  string
	ParentID  *int64
	Sentiment string // "positive", "negative", or empty for all
	Sort      string // "latest", "top_liked", "most_replies"
	Page      int
	PageSize  int
	UserID    int64 // Calling user ID (to populate UserVote)
}
