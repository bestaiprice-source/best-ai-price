package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProviderCommentRepo struct {
	comments map[int64]*service.ProviderComment
	votes    map[string]string // key: commentID:userID -> voteType
	nextID   int64
}

func newMockProviderCommentRepo() *mockProviderCommentRepo {
	return &mockProviderCommentRepo{
		comments: make(map[int64]*service.ProviderComment),
		votes:    make(map[string]string),
		nextID:   1,
	}
}

func (m *mockProviderCommentRepo) CreateComment(ctx context.Context, c *service.ProviderComment) error {
	c.ID = m.nextID
	m.nextID++
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	copyComment := *c
	m.comments[c.ID] = &copyComment
	return nil
}

func (m *mockProviderCommentRepo) GetByID(ctx context.Context, id int64) (*service.ProviderComment, error) {
	c, ok := m.comments[id]
	if !ok {
		return nil, service.ErrCommentNotFound
	}
	copyComment := *c
	return &copyComment, nil
}

func (m *mockProviderCommentRepo) ListProviderComments(ctx context.Context, query service.CommentListQuery) ([]service.ProviderComment, *pagination.PaginationResult, error) {
	var list []service.ProviderComment
	for _, c := range m.comments {
		if c.Provider == query.Provider && c.ParentID == nil && c.Status == service.CommentStatusActive {
			if query.Sentiment == "" || c.Sentiment == query.Sentiment {
				copyComment := *c
				if query.UserID > 0 {
					key := voteKey(c.ID, query.UserID)
					copyComment.UserVote = m.votes[key]
				}
				list = append(list, copyComment)
			}
		}
	}
	total := len(list)
	return list, &pagination.PaginationResult{Total: int64(total), Page: query.Page, PageSize: query.PageSize}, nil
}

func (m *mockProviderCommentRepo) ListReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]service.ProviderComment, *pagination.PaginationResult, error) {
	var list []service.ProviderComment
	for _, c := range m.comments {
		if c.ParentID != nil && *c.ParentID == parentID && c.Status == service.CommentStatusActive {
			copyComment := *c
			if currentUserID > 0 {
				key := voteKey(c.ID, currentUserID)
				copyComment.UserVote = m.votes[key]
			}
			list = append(list, copyComment)
		}
	}
	total := len(list)
	return list, &pagination.PaginationResult{Total: int64(total), Page: page, PageSize: pageSize}, nil
}

func (m *mockProviderCommentRepo) GetProviderSummary(ctx context.Context, provider string) (*service.ProviderCommentSummary, error) {
	total, pos, neg := 0, 0, 0
	for _, c := range m.comments {
		if c.Provider == provider && c.ParentID == nil && c.Status == service.CommentStatusActive {
			total++
			if c.Sentiment == service.SentimentPositive {
				pos++
			} else if c.Sentiment == service.SentimentNegative {
				neg++
			}
		}
	}
	ratio := 0.0
	if total > 0 {
		ratio = float64(pos) / float64(total) * 100
	}
	return &service.ProviderCommentSummary{
		Provider:      provider,
		TotalComments: total,
		PositiveCount: pos,
		NegativeCount: neg,
		PositiveRatio: ratio,
	}, nil
}

func (m *mockProviderCommentRepo) IncrementReplyCount(ctx context.Context, commentID int64) error {
	if c, ok := m.comments[commentID]; ok {
		c.ReplyCount++
	}
	return nil
}

func (m *mockProviderCommentRepo) SetVote(ctx context.Context, commentID, userID int64, voteType string) (*service.VoteResult, error) {
	c, ok := m.comments[commentID]
	if !ok {
		return nil, service.ErrCommentNotFound
	}
	key := voteKey(commentID, userID)
	existing := m.votes[key]

	if existing != "" {
		if existing == service.VoteTypeLike {
			c.LikeCount--
		} else if existing == service.VoteTypeDislike {
			c.DislikeCount--
		}
		delete(m.votes, key)
	}

	finalVote := service.VoteTypeNone
	if voteType != service.VoteTypeNone && voteType != existing {
		if voteType == service.VoteTypeLike {
			c.LikeCount++
		} else if voteType == service.VoteTypeDislike {
			c.DislikeCount++
		}
		m.votes[key] = voteType
		finalVote = voteType
	}

	if c.LikeCount < 0 {
		c.LikeCount = 0
	}
	if c.DislikeCount < 0 {
		c.DislikeCount = 0
	}

	return &service.VoteResult{
		CommentID:    commentID,
		LikeCount:    c.LikeCount,
		DislikeCount: c.DislikeCount,
		UserVote:     finalVote,
	}, nil
}

func (m *mockProviderCommentRepo) GetUserVotes(ctx context.Context, userID int64, commentIDs []int64) (map[int64]string, error) {
	res := make(map[int64]string)
	for _, cid := range commentIDs {
		key := voteKey(cid, userID)
		if v, ok := m.votes[key]; ok {
			res[cid] = v
		}
	}
	return res, nil
}

func (m *mockProviderCommentRepo) AdminListComments(ctx context.Context, provider, status, sentiment string, page, pageSize int) ([]service.ProviderComment, *pagination.PaginationResult, error) {
	var list []service.ProviderComment
	for _, c := range m.comments {
		if (provider == "" || c.Provider == provider) && (status == "" || c.Status == status) && (sentiment == "" || c.Sentiment == sentiment) {
			list = append(list, *c)
		}
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: page, PageSize: pageSize}, nil
}

func (m *mockProviderCommentRepo) AdminSetStatus(ctx context.Context, id int64, status string) error {
	c, ok := m.comments[id]
	if !ok {
		return service.ErrCommentNotFound
	}
	c.Status = status
	return nil
}

func voteKey(commentID, userID int64) string {
	return strings.Join([]string{strings.TrimSpace(string(rune(commentID))), strings.TrimSpace(string(rune(userID)))}, ":")
}

func TestProviderCommentService(t *testing.T) {
	repo := newMockProviderCommentRepo()
	svc := service.NewProviderCommentService(repo)
	ctx := context.Background()

	t.Run("Create root positive review", func(t *testing.T) {
		comment, err := svc.CreateComment(ctx, service.CreateCommentInput{
			Provider:  "yunwu",
			UserID:    101,
			Content:   "Yunwu provider is super fast and stable!",
			Sentiment: "positive",
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), comment.ID)
		assert.Equal(t, "yunwu", comment.Provider)
		assert.Equal(t, int64(101), comment.UserID)
		assert.Equal(t, domain.SentimentPositive, comment.Sentiment)
		assert.Nil(t, comment.ParentID)
	})

	t.Run("Create root negative review", func(t *testing.T) {
		comment, err := svc.CreateComment(ctx, service.CreateCommentInput{
			Provider:  "yunwu",
			UserID:    102,
			Content:   "Encountered occasional 429 rate limits during peak hours.",
			Sentiment: "negative",
		})
		require.NoError(t, err)
		assert.Equal(t, int64(2), comment.ID)
		assert.Equal(t, domain.SentimentNegative, comment.Sentiment)
	})

	t.Run("Validation errors", func(t *testing.T) {
		_, err := svc.CreateComment(ctx, service.CreateCommentInput{
			Provider: "yunwu",
			UserID:   101,
			Content:  "",
		})
		assert.ErrorIs(t, err, service.ErrCommentContentEmpty)

		_, err = svc.CreateComment(ctx, service.CreateCommentInput{
			Provider: "yunwu",
			UserID:   101,
			Content:  strings.Repeat("a", 5001),
		})
		assert.ErrorIs(t, err, service.ErrCommentContentTooLong)

		invalidParent := int64(9999)
		_, err = svc.CreateComment(ctx, service.CreateCommentInput{
			UserID:   101,
			ParentID: &invalidParent,
			Content:  "reply to missing",
		})
		assert.ErrorIs(t, err, service.ErrParentCommentNotFound)
	})

	t.Run("Create reply to comment", func(t *testing.T) {
		parentID := int64(1)
		reply, err := svc.CreateComment(ctx, service.CreateCommentInput{
			UserID:   103,
			ParentID: &parentID,
			Content:  "I agree, it handles high concurrency well!",
		})
		require.NoError(t, err)
		assert.Equal(t, int64(3), reply.ID)
		assert.Equal(t, &parentID, reply.ParentID)
		assert.Equal(t, "yunwu", reply.Provider)
		assert.Equal(t, domain.SentimentNeutral, reply.Sentiment)

		// Check parent reply count incremented
		parent, err := repo.GetByID(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, 1, parent.ReplyCount)
	})

	t.Run("List comments and summary", func(t *testing.T) {
		comments, pag, summary, err := svc.ListComments(ctx, service.CommentListQuery{
			Provider: "yunwu",
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.Len(t, comments, 2)
		assert.Equal(t, int64(2), pag.Total)
		assert.Equal(t, 2, summary.TotalComments)
		assert.Equal(t, 1, summary.PositiveCount)
		assert.Equal(t, 1, summary.NegativeCount)
		assert.Equal(t, 50.0, summary.PositiveRatio)
	})

	t.Run("List replies", func(t *testing.T) {
		replies, pag, err := svc.ListReplies(ctx, 1, 1, 10, 103)
		require.NoError(t, err)
		assert.Len(t, replies, 1)
		assert.Equal(t, int64(1), pag.Total)
		assert.Equal(t, "I agree, it handles high concurrency well!", replies[0].Content)

		_, _, err = svc.ListReplies(ctx, 9999, 1, 10, 0)
		assert.ErrorIs(t, err, service.ErrCommentNotFound)
	})

	t.Run("Vote comment - like, dislike, cancel", func(t *testing.T) {
		// 1. User 101 likes comment 1
		res, err := svc.VoteComment(ctx, service.VoteCommentInput{
			CommentID: 1,
			UserID:    101,
			VoteType:  "like",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, res.LikeCount)
		assert.Equal(t, 0, res.DislikeCount)
		assert.Equal(t, "like", res.UserVote)

		// 2. User 101 changes vote to dislike
		res, err = svc.VoteComment(ctx, service.VoteCommentInput{
			CommentID: 1,
			UserID:    101,
			VoteType:  "dislike",
		})
		require.NoError(t, err)
		assert.Equal(t, 0, res.LikeCount)
		assert.Equal(t, 1, res.DislikeCount)
		assert.Equal(t, "dislike", res.UserVote)

		// 3. User 101 removes dislike (cancels vote)
		res, err = svc.VoteComment(ctx, service.VoteCommentInput{
			CommentID: 1,
			UserID:    101,
			VoteType:  "none",
		})
		require.NoError(t, err)
		assert.Equal(t, 0, res.LikeCount)
		assert.Equal(t, 0, res.DislikeCount)
		assert.Equal(t, "none", res.UserVote)

		// 4. Invalid vote type
		_, err = svc.VoteComment(ctx, service.VoteCommentInput{
			CommentID: 1,
			UserID:    101,
			VoteType:  "invalid_vote",
		})
		assert.ErrorIs(t, err, service.ErrInvalidVoteType)

		// 5. Vote on non-existent comment
		_, err = svc.VoteComment(ctx, service.VoteCommentInput{
			CommentID: 9999,
			UserID:    101,
			VoteType:  "like",
		})
		assert.ErrorIs(t, err, service.ErrCommentNotFound)
	})

	t.Run("Admin moderation", func(t *testing.T) {
		err := svc.AdminSetStatus(ctx, 1, service.CommentStatusHidden)
		require.NoError(t, err)

		c, err := repo.GetByID(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, service.CommentStatusHidden, c.Status)

		adminList, _, err := svc.AdminListComments(ctx, "yunwu", service.CommentStatusHidden, "", 1, 10)
		require.NoError(t, err)
		assert.Len(t, adminList, 1)
	})

	t.Run("GetProviderSummary and pagination bounds", func(t *testing.T) {
		summary, err := svc.GetProviderSummary(ctx, "yunwu")
		require.NoError(t, err)
		assert.Equal(t, "yunwu", summary.Provider)

		// Test pagination bounds
		_, pag, _, err := svc.ListComments(ctx, service.CommentListQuery{
			Provider: "yunwu",
			Page:     0,
			PageSize: 200,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, pag.Page)
		assert.Equal(t, 100, pag.PageSize)

		// Test admin pagination bounds
		_, adminPag, err := svc.AdminListComments(ctx, "yunwu", "", "", 0, 500)
		require.NoError(t, err)
		assert.Equal(t, 1, adminPag.Page)
		assert.Equal(t, 100, adminPag.PageSize)
	})
}
