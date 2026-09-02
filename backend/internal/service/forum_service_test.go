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
	posts  map[int64]*service.ForumPost
	votes  map[string]string // key: postID:userID -> voteType
	nextID int64
}

func newMockForumRepo() *mockForumRepo {
	return &mockForumRepo{
		posts:  make(map[int64]*service.ForumPost),
		votes:  make(map[string]string),
		nextID: 1,
	}
}

func forumVoteKey(postID, userID int64) string {
	return fmt.Sprintf("%d:%d", postID, userID)
}

func (m *mockForumRepo) CreatePost(ctx context.Context, p *service.ForumPost) error {
	p.ID = m.nextID
	m.nextID++
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	copyPost := *p
	m.posts[p.ID] = &copyPost
	return nil
}

func (m *mockForumRepo) GetByID(ctx context.Context, id int64) (*service.ForumPost, error) {
	p, ok := m.posts[id]
	if !ok {
		return nil, service.ErrForumPostNotFound
	}
	copyPost := *p
	return &copyPost, nil
}

func (m *mockForumRepo) ListPosts(ctx context.Context, query service.ForumPostListQuery) ([]service.ForumPost, *pagination.PaginationResult, error) {
	var list []service.ForumPost
	for _, p := range m.posts {
		if p.ParentID == nil && p.Status == service.ForumPostStatusActive {
			copyPost := *p
			if query.UserID > 0 {
				key := forumVoteKey(p.ID, query.UserID)
				copyPost.UserVote = m.votes[key]
			}
			list = append(list, copyPost)
		}
	}
	total := len(list)
	return list, &pagination.PaginationResult{Total: int64(total), Page: query.Page, PageSize: query.PageSize}, nil
}

func (m *mockForumRepo) ListReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]service.ForumPost, *pagination.PaginationResult, error) {
	var list []service.ForumPost
	for _, p := range m.posts {
		if p.ParentID != nil && *p.ParentID == parentID && p.Status == service.ForumPostStatusActive {
			copyPost := *p
			if currentUserID > 0 {
				key := forumVoteKey(p.ID, currentUserID)
				copyPost.UserVote = m.votes[key]
			}
			list = append(list, copyPost)
		}
	}
	total := len(list)
	return list, &pagination.PaginationResult{Total: int64(total), Page: page, PageSize: pageSize}, nil
}

func (m *mockForumRepo) IncrementReplyCount(ctx context.Context, postID int64) error {
	if p, ok := m.posts[postID]; ok {
		p.ReplyCount++
	}
	return nil
}

func (m *mockForumRepo) SetVote(ctx context.Context, postID, userID int64, voteType string) (*service.ForumVoteResult, error) {
	p, ok := m.posts[postID]
	if !ok {
		return nil, service.ErrForumPostNotFound
	}
	key := forumVoteKey(postID, userID)
	prev := m.votes[key]

	if prev == service.ForumVoteTypeLike {
		p.LikeCount--
	} else if prev == service.ForumVoteTypeDislike {
		p.DislikeCount--
	}

	if voteType == service.ForumVoteTypeLike {
		p.LikeCount++
		m.votes[key] = service.ForumVoteTypeLike
	} else if voteType == service.ForumVoteTypeDislike {
		p.DislikeCount++
		m.votes[key] = service.ForumVoteTypeDislike
	} else {
		delete(m.votes, key)
		voteType = service.ForumVoteTypeNone
	}

	if p.LikeCount < 0 {
		p.LikeCount = 0
	}
	if p.DislikeCount < 0 {
		p.DislikeCount = 0
	}

	return &service.ForumVoteResult{
		PostID:       postID,
		LikeCount:    p.LikeCount,
		DislikeCount: p.DislikeCount,
		UserVote:     voteType,
	}, nil
}

func (m *mockForumRepo) GetUserVotes(ctx context.Context, userID int64, postIDs []int64) (map[int64]string, error) {
	res := make(map[int64]string)
	for _, id := range postIDs {
		key := forumVoteKey(id, userID)
		if v, ok := m.votes[key]; ok {
			res[id] = v
		}
	}
	return res, nil
}

func (m *mockForumRepo) AdminListPosts(ctx context.Context, status string, page, pageSize int) ([]service.ForumPost, *pagination.PaginationResult, error) {
	var list []service.ForumPost
	for _, p := range m.posts {
		if status == "" || p.Status == status {
			list = append(list, *p)
		}
	}
	return list, &pagination.PaginationResult{Total: int64(len(list)), Page: page, PageSize: pageSize}, nil
}

func (m *mockForumRepo) AdminSetStatus(ctx context.Context, id int64, status string) error {
	p, ok := m.posts[id]
	if !ok {
		return service.ErrForumPostNotFound
	}
	p.Status = status
	return nil
}

func TestForumService_CreatePost(t *testing.T) {
	ctx := context.Background()
	repo := newMockForumRepo()
	svc := service.NewForumService(repo)

	t.Run("success create root post", func(t *testing.T) {
		post, err := svc.CreatePost(ctx, service.CreateForumPostInput{
			UserID:  10,
			Content: "Hello world this is my first forum post",
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), post.ID)
		assert.Equal(t, int64(10), post.UserID)
		assert.Equal(t, "Hello world this is my first forum post", post.Content)
		assert.Equal(t, service.ForumPostStatusActive, post.Status)
		assert.Nil(t, post.ParentID)
		assert.Equal(t, 0, post.LikeCount)
		assert.Equal(t, 0, post.DislikeCount)
		assert.Equal(t, 0, post.ReplyCount)
	})

	t.Run("empty content error", func(t *testing.T) {
		_, err := svc.CreatePost(ctx, service.CreateForumPostInput{
			UserID:  10,
			Content: "   ",
		})
		assert.ErrorIs(t, err, service.ErrForumContentEmpty)
	})

	t.Run("content too long error", func(t *testing.T) {
		longContent := strings.Repeat("a", 5001)
		_, err := svc.CreatePost(ctx, service.CreateForumPostInput{
			UserID:  10,
			Content: longContent,
		})
		assert.ErrorIs(t, err, service.ErrForumContentTooLong)
	})

	t.Run("reply to non-existent parent error", func(t *testing.T) {
		parentID := int64(999)
		_, err := svc.CreatePost(ctx, service.CreateForumPostInput{
			UserID:   10,
			ParentID: &parentID,
			Content:  "Replying to ghost post",
		})
		assert.ErrorIs(t, err, service.ErrForumParentNotFound)
	})

	t.Run("success reply to existing post increments parent reply_count", func(t *testing.T) {
		parentID := int64(1)
		reply, err := svc.CreatePost(ctx, service.CreateForumPostInput{
			UserID:   11,
			ParentID: &parentID,
			Content:  "I reply to post 1",
		})
		require.NoError(t, err)
		assert.Equal(t, int64(2), reply.ID)
		assert.Equal(t, &parentID, reply.ParentID)

		parent, err := repo.GetByID(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, 1, parent.ReplyCount)
	})

	t.Run("nested reply to reply", func(t *testing.T) {
		replyParentID := int64(2)
		nestedReply, err := svc.CreatePost(ctx, service.CreateForumPostInput{
			UserID:   12,
			ParentID: &replyParentID,
			Content:  "I reply to reply 2",
		})
		require.NoError(t, err)
		assert.Equal(t, int64(3), nestedReply.ID)
		assert.Equal(t, &replyParentID, nestedReply.ParentID)

		parentReply, err := repo.GetByID(ctx, 2)
		require.NoError(t, err)
		assert.Equal(t, 1, parentReply.ReplyCount)
	})
}

func TestForumService_ListPosts(t *testing.T) {
	ctx := context.Background()
	repo := newMockForumRepo()
	svc := service.NewForumService(repo)

	// Create root post and replies
	_, _ = svc.CreatePost(ctx, service.CreateForumPostInput{UserID: 1, Content: "Post 1"})
	parentID := int64(1)
	_, _ = svc.CreatePost(ctx, service.CreateForumPostInput{UserID: 2, ParentID: &parentID, Content: "Reply to Post 1"})
	_, _ = svc.CreatePost(ctx, service.CreateForumPostInput{UserID: 3, Content: "Post 2"})

	t.Run("list root posts", func(t *testing.T) {
		posts, pag, err := svc.ListPosts(ctx, service.ForumPostListQuery{
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, len(posts))
		assert.Equal(t, int64(2), pag.Total)
	})

	t.Run("list root posts with page defaults clamping", func(t *testing.T) {
		posts, pag, err := svc.ListPosts(ctx, service.ForumPostListQuery{
			Page:     0,
			PageSize: 150,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, len(posts))
		assert.Equal(t, 100, pag.PageSize)
	})
}

func TestForumService_ListReplies(t *testing.T) {
	ctx := context.Background()
	repo := newMockForumRepo()
	svc := service.NewForumService(repo)

	p, _ := svc.CreatePost(ctx, service.CreateForumPostInput{UserID: 1, Content: "Root"})
	_, _ = svc.CreatePost(ctx, service.CreateForumPostInput{UserID: 2, ParentID: &p.ID, Content: "Reply 1"})
	_, _ = svc.CreatePost(ctx, service.CreateForumPostInput{UserID: 3, ParentID: &p.ID, Content: "Reply 2"})

	t.Run("list replies success", func(t *testing.T) {
		replies, pag, err := svc.ListReplies(ctx, p.ID, 1, 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 2, len(replies))
		assert.Equal(t, int64(2), pag.Total)
	})

	t.Run("list replies with page defaults and clamping", func(t *testing.T) {
		replies, pag, err := svc.ListReplies(ctx, p.ID, 0, 200, 0)
		require.NoError(t, err)
		assert.Equal(t, 2, len(replies))
		assert.Equal(t, 100, pag.PageSize)
	})

	t.Run("list replies parent not found", func(t *testing.T) {
		_, _, err := svc.ListReplies(ctx, 999, 1, 10, 0)
		assert.ErrorIs(t, err, service.ErrForumPostNotFound)
	})
}

func TestForumService_VotePost(t *testing.T) {
	ctx := context.Background()
	repo := newMockForumRepo()
	svc := service.NewForumService(repo)

	post, _ := svc.CreatePost(ctx, service.CreateForumPostInput{UserID: 1, Content: "Vote Target"})

	t.Run("invalid vote type", func(t *testing.T) {
		_, err := svc.VotePost(ctx, service.VoteForumPostInput{
			PostID:   post.ID,
			UserID:   2,
			VoteType: "invalid",
		})
		assert.ErrorIs(t, err, service.ErrForumInvalidVoteType)
	})

	t.Run("vote post not found", func(t *testing.T) {
		_, err := svc.VotePost(ctx, service.VoteForumPostInput{
			PostID:   999,
			UserID:   2,
			VoteType: "like",
		})
		assert.ErrorIs(t, err, service.ErrForumPostNotFound)
	})

	t.Run("vote like then dislike then clear", func(t *testing.T) {
		// 1. Like
		res, err := svc.VotePost(ctx, service.VoteForumPostInput{
			PostID:   post.ID,
			UserID:   2,
			VoteType: "like",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, res.LikeCount)
		assert.Equal(t, 0, res.DislikeCount)
		assert.Equal(t, "like", res.UserVote)

		// 2. Dislike (switch)
		res, err = svc.VotePost(ctx, service.VoteForumPostInput{
			PostID:   post.ID,
			UserID:   2,
			VoteType: "dislike",
		})
		require.NoError(t, err)
		assert.Equal(t, 0, res.LikeCount)
		assert.Equal(t, 1, res.DislikeCount)
		assert.Equal(t, "dislike", res.UserVote)

		// 3. None (cancel)
		res, err = svc.VotePost(ctx, service.VoteForumPostInput{
			PostID:   post.ID,
			UserID:   2,
			VoteType: "none",
		})
		require.NoError(t, err)
		assert.Equal(t, 0, res.LikeCount)
		assert.Equal(t, 0, res.DislikeCount)
		assert.Equal(t, "none", res.UserVote)
	})

	t.Run("vote on hidden post returns not found", func(t *testing.T) {
		hiddenPost, _ := svc.CreatePost(ctx, service.CreateForumPostInput{UserID: 1, Content: "Hidden"})
		_ = repo.AdminSetStatus(ctx, hiddenPost.ID, service.ForumPostStatusHidden)

		_, err := svc.VotePost(ctx, service.VoteForumPostInput{
			PostID:   hiddenPost.ID,
			UserID:   2,
			VoteType: "like",
		})
		assert.ErrorIs(t, err, service.ErrForumPostNotFound)
	})
}

func TestForumService_AdminOperations(t *testing.T) {
	ctx := context.Background()
	repo := newMockForumRepo()
	svc := service.NewForumService(repo)

	p1, _ := svc.CreatePost(ctx, service.CreateForumPostInput{UserID: 1, Content: "Admin Test 1"})
	p2, _ := svc.CreatePost(ctx, service.CreateForumPostInput{UserID: 2, Content: "Admin Test 2"})

	t.Run("admin set status", func(t *testing.T) {
		err := svc.AdminSetStatus(ctx, p1.ID, service.ForumPostStatusHidden)
		require.NoError(t, err)

		fetched, err := repo.GetByID(ctx, p1.ID)
		require.NoError(t, err)
		assert.Equal(t, service.ForumPostStatusHidden, fetched.Status)
	})

	t.Run("admin set invalid status", func(t *testing.T) {
		err := svc.AdminSetStatus(ctx, p2.ID, "unknown_status")
		assert.ErrorIs(t, err, service.ErrForumPostNotFound)
	})

	t.Run("admin list posts with filter", func(t *testing.T) {
		list, pag, err := svc.AdminListPosts(ctx, service.ForumPostStatusHidden, 1, 10)
		require.NoError(t, err)
		assert.Equal(t, 1, len(list))
		assert.Equal(t, int64(1), pag.Total)
	})
}

func TestNormalizeForumVoteType(t *testing.T) {
	assert.Equal(t, "like", domain.NormalizeForumVoteType("LIKE"))
	assert.Equal(t, "dislike", domain.NormalizeForumVoteType("  DisLike  "))
	assert.Equal(t, "none", domain.NormalizeForumVoteType("none"))
	assert.Equal(t, "none", domain.NormalizeForumVoteType(""))
	assert.Equal(t, "", domain.NormalizeForumVoteType("invalid"))
}
