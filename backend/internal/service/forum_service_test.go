package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockForumRepo struct {
	topics       map[int64]*service.ForumTopic
	comments     map[int64]*service.ForumComment
	topicVotes   map[string]string // topicID:userID -> voteType
	commentVotes map[string]string // commentID:userID -> voteType
	nextTopicID  int64
	nextCommentID int64
}

func newMockForumRepo() *mockForumRepo {
	return &mockForumRepo{
		topics:       make(map[int64]*service.ForumTopic),
		comments:     make(map[int64]*service.ForumComment),
		topicVotes:   make(map[string]string),
		commentVotes: make(map[string]string),
		nextTopicID:  1,
		nextCommentID: 1,
	}
}

func forumVoteKey(id, userID int64) string {
	return fmt.Sprintf("%d:%d", id, userID)
}

func (m *mockForumRepo) CreateTopic(ctx context.Context, t *service.ForumTopic) error {
	t.ID = m.nextTopicID
	m.nextTopicID++
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	copyTopic := *t
	m.topics[t.ID] = &copyTopic
	return nil
}

func (m *mockForumRepo) GetTopicByID(ctx context.Context, id int64) (*service.ForumTopic, error) {
	t, ok := m.topics[id]
	if !ok {
		return nil, service.ErrForumTopicNotFound
	}
	copyTopic := *t
	return &copyTopic, nil
}

func (m *mockForumRepo) ListTopics(ctx context.Context, query service.ForumTopicListQuery) ([]service.ForumTopic, *pagination.PaginationResult, error) {
	var list []service.ForumTopic
	for _, t := range m.topics {
		if t.Status == service.ForumTopicStatusActive {
			if query.Search != "" && !strings.Contains(strings.ToLower(t.Title), strings.ToLower(query.Search)) {
				continue
			}
			copyTopic := *t
			if query.UserID > 0 {
				key := forumVoteKey(t.ID, query.UserID)
				copyTopic.UserVote = m.topicVotes[key]
			}
			list = append(list, copyTopic)
		}
	}
	total := len(list)
	return list, &pagination.PaginationResult{Total: int64(total), Page: query.Page, PageSize: query.PageSize}, nil
}

func (m *mockForumRepo) SetTopicVote(ctx context.Context, topicID, userID int64, voteType string) (*service.ForumVoteResult, error) {
	t, ok := m.topics[topicID]
	if !ok {
		return nil, service.ErrForumTopicNotFound
	}
	key := forumVoteKey(topicID, userID)
	prev := m.topicVotes[key]

	if prev == service.ForumVoteTypeLike {
		t.LikeCount--
	} else if prev == service.ForumVoteTypeDislike {
		t.DislikeCount--
	}

	if voteType == service.ForumVoteTypeLike {
		t.LikeCount++
		m.topicVotes[key] = service.ForumVoteTypeLike
	} else if voteType == service.ForumVoteTypeDislike {
		t.DislikeCount++
		m.topicVotes[key] = service.ForumVoteTypeDislike
	} else {
		delete(m.topicVotes, key)
		voteType = service.ForumVoteTypeNone
	}

	if t.LikeCount < 0 {
		t.LikeCount = 0
	}
	if t.DislikeCount < 0 {
		t.DislikeCount = 0
	}

	return &service.ForumVoteResult{
		ID:           topicID,
		LikeCount:    t.LikeCount,
		DislikeCount: t.DislikeCount,
		UserVote:     voteType,
	}, nil
}

func (m *mockForumRepo) GetTopicUserVotes(ctx context.Context, userID int64, topicIDs []int64) (map[int64]string, error) {
	res := make(map[int64]string)
	for _, id := range topicIDs {
		key := forumVoteKey(id, userID)
		if v, ok := m.topicVotes[key]; ok {
			res[id] = v
		}
	}
	return res, nil
}

func (m *mockForumRepo) AdminListTopics(ctx context.Context, status string, page, pageSize int) ([]service.ForumTopic, *pagination.PaginationResult, error) {
	var list []service.ForumTopic
	for _, t := range m.topics {
		if status == "" || t.Status == status {
			list = append(list, *t)
		}
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: page, PageSize: pageSize}, nil
}

func (m *mockForumRepo) AdminSetTopicStatus(ctx context.Context, id int64, status string) error {
	t, ok := m.topics[id]
	if !ok {
		return service.ErrForumTopicNotFound
	}
	t.Status = status
	return nil
}

func (m *mockForumRepo) CreateComment(ctx context.Context, c *service.ForumComment) error {
	c.ID = m.nextCommentID
	m.nextCommentID++
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	copyComment := *c
	m.comments[c.ID] = &copyComment
	return nil
}

func (m *mockForumRepo) GetCommentByID(ctx context.Context, id int64) (*service.ForumComment, error) {
	c, ok := m.comments[id]
	if !ok {
		return nil, service.ErrForumCommentNotFound
	}
	copyComment := *c
	return &copyComment, nil
}

func (m *mockForumRepo) ListCommentsByTopic(ctx context.Context, query service.ForumCommentListQuery) ([]service.ForumComment, *pagination.PaginationResult, error) {
	var list []service.ForumComment
	for _, c := range m.comments {
		if c.TopicID == query.TopicID && c.ParentID == nil && c.Status == service.ForumCommentStatusActive {
			copyComment := *c
			if query.UserID > 0 {
				key := forumVoteKey(c.ID, query.UserID)
				copyComment.UserVote = m.commentVotes[key]
			}
			list = append(list, copyComment)
		}
	}
	total := len(list)
	return list, &pagination.PaginationResult{Total: int64(total), Page: query.Page, PageSize: query.PageSize}, nil
}

func (m *mockForumRepo) ListCommentReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]service.ForumComment, *pagination.PaginationResult, error) {
	var list []service.ForumComment
	for _, c := range m.comments {
		if c.ParentID != nil && *c.ParentID == parentID && c.Status == service.ForumCommentStatusActive {
			copyComment := *c
			if currentUserID > 0 {
				key := forumVoteKey(c.ID, currentUserID)
				copyComment.UserVote = m.commentVotes[key]
			}
			list = append(list, copyComment)
		}
	}
	total := len(list)
	return list, &pagination.PaginationResult{Total: int64(total), Page: page, PageSize: pageSize}, nil
}

func (m *mockForumRepo) IncrementTopicCommentCount(ctx context.Context, topicID int64) error {
	if t, ok := m.topics[topicID]; ok {
		t.CommentCount++
	}
	return nil
}

func (m *mockForumRepo) IncrementCommentReplyCount(ctx context.Context, commentID int64) error {
	if c, ok := m.comments[commentID]; ok {
		c.ReplyCount++
	}
	return nil
}

func (m *mockForumRepo) SetCommentVote(ctx context.Context, commentID, userID int64, voteType string) (*service.ForumVoteResult, error) {
	c, ok := m.comments[commentID]
	if !ok {
		return nil, service.ErrForumCommentNotFound
	}
	key := forumVoteKey(commentID, userID)
	prev := m.commentVotes[key]

	if prev == service.ForumVoteTypeLike {
		c.LikeCount--
	} else if prev == service.ForumVoteTypeDislike {
		c.DislikeCount--
	}

	if voteType == service.ForumVoteTypeLike {
		c.LikeCount++
		m.commentVotes[key] = service.ForumVoteTypeLike
	} else if voteType == service.ForumVoteTypeDislike {
		c.DislikeCount++
		m.commentVotes[key] = service.ForumVoteTypeDislike
	} else {
		delete(m.commentVotes, key)
		voteType = service.ForumVoteTypeNone
	}

	if c.LikeCount < 0 {
		c.LikeCount = 0
	}
	if c.DislikeCount < 0 {
		c.DislikeCount = 0
	}

	return &service.ForumVoteResult{
		ID:           commentID,
		LikeCount:    c.LikeCount,
		DislikeCount: c.DislikeCount,
		UserVote:     voteType,
	}, nil
}

func (m *mockForumRepo) GetCommentUserVotes(ctx context.Context, userID int64, commentIDs []int64) (map[int64]string, error) {
	res := make(map[int64]string)
	for _, id := range commentIDs {
		key := forumVoteKey(id, userID)
		if v, ok := m.commentVotes[key]; ok {
			res[id] = v
		}
	}
	return res, nil
}

func (m *mockForumRepo) AdminListComments(ctx context.Context, status string, page, pageSize int) ([]service.ForumComment, *pagination.PaginationResult, error) {
	var list []service.ForumComment
	for _, c := range m.comments {
		if status == "" || c.Status == status {
			list = append(list, *c)
		}
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: page, PageSize: pageSize}, nil
}

func (m *mockForumRepo) AdminSetCommentStatus(ctx context.Context, id int64, status string) error {
	c, ok := m.comments[id]
	if !ok {
		return service.ErrForumCommentNotFound
	}
	c.Status = status
	return nil
}

// -----------------------------------------------------------------------------
// Tests
// -----------------------------------------------------------------------------

func TestForumService_Topics(t *testing.T) {
	ctx := context.Background()
	repo := newMockForumRepo()
	svc := service.NewForumService(repo)

	t.Run("create topic success", func(t *testing.T) {
		topic, err := svc.CreateTopic(ctx, service.CreateForumTopicInput{
			UserID:  1,
			Title:   "First Topic",
			Content: "This is the topic description",
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), topic.ID)
		assert.Equal(t, "First Topic", topic.Title)
		assert.Equal(t, "This is the topic description", topic.Content)
		assert.Equal(t, service.ForumTopicStatusActive, topic.Status)
		assert.Equal(t, 0, topic.LikeCount)
		assert.Equal(t, 0, topic.CommentCount)
	})

	t.Run("create topic empty title", func(t *testing.T) {
		_, err := svc.CreateTopic(ctx, service.CreateForumTopicInput{
			UserID:  1,
			Title:   "   ",
			Content: "Valid content",
		})
		assert.ErrorIs(t, err, service.ErrForumTitleEmpty)
	})

	t.Run("create topic title too long", func(t *testing.T) {
		_, err := svc.CreateTopic(ctx, service.CreateForumTopicInput{
			UserID:  1,
			Title:   strings.Repeat("a", 256),
			Content: "Valid content",
		})
		assert.ErrorIs(t, err, service.ErrForumTitleTooLong)
	})

	t.Run("create topic empty content", func(t *testing.T) {
		_, err := svc.CreateTopic(ctx, service.CreateForumTopicInput{
			UserID:  1,
			Title:   "Valid title",
			Content: "   ",
		})
		assert.ErrorIs(t, err, service.ErrForumContentEmpty)
	})

	t.Run("create topic content too long", func(t *testing.T) {
		_, err := svc.CreateTopic(ctx, service.CreateForumTopicInput{
			UserID:  1,
			Title:   "Valid title",
			Content: strings.Repeat("a", 10001),
		})
		assert.ErrorIs(t, err, service.ErrForumContentTooLong)
	})

	t.Run("get topic success", func(t *testing.T) {
		topic, err := svc.GetTopic(ctx, 1, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(1), topic.ID)
	})

	t.Run("get topic not found", func(t *testing.T) {
		_, err := svc.GetTopic(ctx, 999, 0)
		assert.ErrorIs(t, err, service.ErrForumTopicNotFound)
	})

	t.Run("list topics with search and page clamping", func(t *testing.T) {
		_, _ = svc.CreateTopic(ctx, service.CreateForumTopicInput{
			UserID:  2,
			Title:   "Second Topic GoLang",
			Content: "GoLang discussion",
		})

		topics, pag, err := svc.ListTopics(ctx, service.ForumTopicListQuery{
			Search:   "golang",
			Page:     0,
			PageSize: 150,
		})
		require.NoError(t, err)
		assert.Len(t, topics, 1)
		assert.Equal(t, 100, pag.PageSize)
	})

	t.Run("vote topic like, dislike, cancel", func(t *testing.T) {
		// Like
		res, err := svc.VoteTopic(ctx, service.VoteForumTopicInput{
			TopicID:  1,
			UserID:   10,
			VoteType: "like",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, res.LikeCount)
		assert.Equal(t, 0, res.DislikeCount)
		assert.Equal(t, "like", res.UserVote)

		// Dislike (switch)
		res, err = svc.VoteTopic(ctx, service.VoteForumTopicInput{
			TopicID:  1,
			UserID:   10,
			VoteType: "dislike",
		})
		require.NoError(t, err)
		assert.Equal(t, 0, res.LikeCount)
		assert.Equal(t, 1, res.DislikeCount)
		assert.Equal(t, "dislike", res.UserVote)

		// None (cancel)
		res, err = svc.VoteTopic(ctx, service.VoteForumTopicInput{
			TopicID:  1,
			UserID:   10,
			VoteType: "none",
		})
		require.NoError(t, err)
		assert.Equal(t, 0, res.LikeCount)
		assert.Equal(t, 0, res.DislikeCount)
		assert.Equal(t, "none", res.UserVote)
	})

	t.Run("vote topic invalid vote type", func(t *testing.T) {
		_, err := svc.VoteTopic(ctx, service.VoteForumTopicInput{
			TopicID:  1,
			UserID:   10,
			VoteType: "bad_type",
		})
		assert.ErrorIs(t, err, service.ErrForumInvalidVoteType)
	})

	t.Run("vote topic not found", func(t *testing.T) {
		_, err := svc.VoteTopic(ctx, service.VoteForumTopicInput{
			TopicID:  999,
			UserID:   10,
			VoteType: "like",
		})
		assert.ErrorIs(t, err, service.ErrForumTopicNotFound)
	})

	t.Run("admin set topic status and list", func(t *testing.T) {
		err := svc.AdminSetTopicStatus(ctx, 1, service.ForumTopicStatusHidden)
		require.NoError(t, err)

		list, pag, err := svc.AdminListTopics(ctx, service.ForumTopicStatusHidden, 1, 10)
		require.NoError(t, err)
		assert.Len(t, list, 1)
		assert.Equal(t, int64(1), pag.Total)

		// restore
		_ = svc.AdminSetTopicStatus(ctx, 1, service.ForumTopicStatusActive)
	})

	t.Run("admin set invalid topic status", func(t *testing.T) {
		err := svc.AdminSetTopicStatus(ctx, 1, "invalid_status")
		assert.ErrorIs(t, err, service.ErrForumTopicNotFound)
	})
}

func TestForumService_Comments(t *testing.T) {
	ctx := context.Background()
	repo := newMockForumRepo()
	svc := service.NewForumService(repo)

	topic, _ := svc.CreateTopic(ctx, service.CreateForumTopicInput{
		UserID:  1,
		Title:   "Topic For Comments",
		Content: "Topic Content",
	})

	t.Run("create comment success", func(t *testing.T) {
		comment, err := svc.CreateComment(ctx, service.CreateForumCommentInput{
			TopicID: topic.ID,
			UserID:  2,
			Content: "Great topic!",
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), comment.ID)
		assert.Equal(t, topic.ID, comment.TopicID)
		assert.Equal(t, "Great topic!", comment.Content)
		assert.Nil(t, comment.ParentID)

		tFetched, _ := repo.GetTopicByID(ctx, topic.ID)
		assert.Equal(t, 1, tFetched.CommentCount)
	})

	t.Run("create comment empty content", func(t *testing.T) {
		_, err := svc.CreateComment(ctx, service.CreateForumCommentInput{
			TopicID: topic.ID,
			UserID:  2,
			Content: "   ",
		})
		assert.ErrorIs(t, err, service.ErrForumContentEmpty)
	})

	t.Run("create comment content too long", func(t *testing.T) {
		_, err := svc.CreateComment(ctx, service.CreateForumCommentInput{
			TopicID: topic.ID,
			UserID:  2,
			Content: strings.Repeat("c", 5001),
		})
		assert.ErrorIs(t, err, service.ErrForumContentTooLong)
	})

	t.Run("create comment topic not found", func(t *testing.T) {
		_, err := svc.CreateComment(ctx, service.CreateForumCommentInput{
			TopicID: 999,
			UserID:  2,
			Content: "Comment",
		})
		assert.ErrorIs(t, err, service.ErrForumTopicNotFound)
	})

	t.Run("create reply to comment success", func(t *testing.T) {
		parentID := int64(1)
		reply, err := svc.CreateComment(ctx, service.CreateForumCommentInput{
			TopicID:  topic.ID,
			UserID:   3,
			ParentID: &parentID,
			Content:  "I reply to comment 1",
		})
		require.NoError(t, err)
		assert.Equal(t, int64(2), reply.ID)
		assert.Equal(t, &parentID, reply.ParentID)

		parentComment, _ := repo.GetCommentByID(ctx, 1)
		assert.Equal(t, 1, parentComment.ReplyCount)
	})

	t.Run("create reply to non-existent parent", func(t *testing.T) {
		parentID := int64(999)
		_, err := svc.CreateComment(ctx, service.CreateForumCommentInput{
			TopicID:  topic.ID,
			UserID:   3,
			ParentID: &parentID,
			Content:  "Ghost reply",
		})
		assert.ErrorIs(t, err, service.ErrForumParentNotFound)
	})

	t.Run("list comments by topic", func(t *testing.T) {
		comments, pag, err := svc.ListCommentsByTopic(ctx, service.ForumCommentListQuery{
			TopicID:  topic.ID,
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.Len(t, comments, 1) // only root comments
		assert.Equal(t, int64(1), pag.Total)
	})

	t.Run("list comments topic not found", func(t *testing.T) {
		_, _, err := svc.ListCommentsByTopic(ctx, service.ForumCommentListQuery{
			TopicID:  999,
			Page:     1,
			PageSize: 10,
		})
		assert.ErrorIs(t, err, service.ErrForumTopicNotFound)
	})

	t.Run("list comment replies", func(t *testing.T) {
		replies, pag, err := svc.ListCommentReplies(ctx, 1, 1, 10, 0)
		require.NoError(t, err)
		assert.Len(t, replies, 1)
		assert.Equal(t, int64(1), pag.Total)
	})

	t.Run("list comment replies parent not found", func(t *testing.T) {
		_, _, err := svc.ListCommentReplies(ctx, 999, 1, 10, 0)
		assert.ErrorIs(t, err, service.ErrForumCommentNotFound)
	})

	t.Run("vote comment like, dislike, cancel", func(t *testing.T) {
		res, err := svc.VoteComment(ctx, service.VoteForumCommentInput{
			CommentID: 1,
			UserID:    10,
			VoteType:  "like",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, res.LikeCount)
		assert.Equal(t, "like", res.UserVote)

		res, err = svc.VoteComment(ctx, service.VoteForumCommentInput{
			CommentID: 1,
			UserID:    10,
			VoteType:  "dislike",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, res.DislikeCount)
		assert.Equal(t, "dislike", res.UserVote)

		res, err = svc.VoteComment(ctx, service.VoteForumCommentInput{
			CommentID: 1,
			UserID:    10,
			VoteType:  "none",
		})
		require.NoError(t, err)
		assert.Equal(t, 0, res.LikeCount)
		assert.Equal(t, 0, res.DislikeCount)
		assert.Equal(t, "none", res.UserVote)
	})

	t.Run("admin set comment status and list", func(t *testing.T) {
		err := svc.AdminSetCommentStatus(ctx, 1, service.ForumCommentStatusHidden)
		require.NoError(t, err)

		list, pag, err := svc.AdminListComments(ctx, service.ForumCommentStatusHidden, 0, 150)
		require.NoError(t, err)
		assert.Len(t, list, 1)
		assert.Equal(t, int64(1), pag.Total)
		assert.Equal(t, 100, pag.PageSize)

		err = svc.AdminSetCommentStatus(ctx, 1, "invalid_status")
		assert.ErrorIs(t, err, service.ErrForumCommentNotFound)

		_ = svc.AdminSetCommentStatus(ctx, 1, service.ForumCommentStatusActive)
	})

	t.Run("get comment by id", func(t *testing.T) {
		c, err := svc.GetCommentByID(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, int64(1), c.ID)

		_, err = svc.GetCommentByID(ctx, 999)
		assert.ErrorIs(t, err, service.ErrForumCommentNotFound)

		// Hidden comment
		_ = repo.AdminSetCommentStatus(ctx, 1, service.ForumCommentStatusHidden)
		_, err = svc.GetCommentByID(ctx, 1)
		assert.ErrorIs(t, err, service.ErrForumCommentNotFound)
		_ = repo.AdminSetCommentStatus(ctx, 1, service.ForumCommentStatusActive)
	})

	t.Run("vote comment errors", func(t *testing.T) {
		_, err := svc.VoteComment(ctx, service.VoteForumCommentInput{
			CommentID: 1,
			UserID:    10,
			VoteType:  "invalid",
		})
		assert.ErrorIs(t, err, service.ErrForumInvalidVoteType)

		_, err = svc.VoteComment(ctx, service.VoteForumCommentInput{
			CommentID: 999,
			UserID:    10,
			VoteType:  "like",
		})
		assert.ErrorIs(t, err, service.ErrForumCommentNotFound)

		_ = repo.AdminSetCommentStatus(ctx, 1, service.ForumCommentStatusHidden)
		_, err = svc.VoteComment(ctx, service.VoteForumCommentInput{
			CommentID: 1,
			UserID:    10,
			VoteType:  "like",
		})
		assert.ErrorIs(t, err, service.ErrForumCommentNotFound)
		_ = repo.AdminSetCommentStatus(ctx, 1, service.ForumCommentStatusActive)
	})

	t.Run("create comment on hidden topic or parent on different topic", func(t *testing.T) {
		hiddenTopic, _ := svc.CreateTopic(ctx, service.CreateForumTopicInput{
			UserID:  1,
			Title:   "Hidden Topic",
			Content: "Hidden Content",
		})
		_ = repo.AdminSetTopicStatus(ctx, hiddenTopic.ID, service.ForumTopicStatusHidden)

		_, err := svc.CreateComment(ctx, service.CreateForumCommentInput{
			TopicID: hiddenTopic.ID,
			UserID:  2,
			Content: "Comment",
		})
		assert.ErrorIs(t, err, service.ErrForumTopicNotFound)

		// Parent comment on different topic
		parentID := int64(1)
		otherTopic, _ := svc.CreateTopic(ctx, service.CreateForumTopicInput{
			UserID:  1,
			Title:   "Other Topic",
			Content: "Other Content",
		})
		_, err = svc.CreateComment(ctx, service.CreateForumCommentInput{
			TopicID:  otherTopic.ID,
			UserID:   2,
			ParentID: &parentID,
			Content:  "Comment",
		})
		assert.ErrorIs(t, err, service.ErrForumParentNotFound)
	})

	t.Run("list comments and replies with page clamping", func(t *testing.T) {
		_, pag, err := svc.ListCommentsByTopic(ctx, service.ForumCommentListQuery{
			TopicID:  topic.ID,
			Page:     0,
			PageSize: 150,
		})
		require.NoError(t, err)
		assert.Equal(t, 100, pag.PageSize)

		_, pagReplies, err := svc.ListCommentReplies(ctx, 1, 0, 150, 0)
		require.NoError(t, err)
		assert.Equal(t, 100, pagReplies.PageSize)
	})
}

func TestNormalizeForumVoteType(t *testing.T) {
	assert.Equal(t, "like", domain.NormalizeForumVoteType("LIKE"))
	assert.Equal(t, "dislike", domain.NormalizeForumVoteType("  DisLike  "))
	assert.Equal(t, "none", domain.NormalizeForumVoteType("none"))
	assert.Equal(t, "none", domain.NormalizeForumVoteType(""))
	assert.Equal(t, "", domain.NormalizeForumVoteType("invalid"))
}
