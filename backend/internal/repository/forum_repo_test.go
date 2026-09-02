package repository

import (
	"context"
	"database/sql"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newForumRepoSQLite(t *testing.T) (service.ForumRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:forum_repo_test?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	repo := NewForumRepository(client)
	return repo, client
}

func TestForumRepository_FullLifecycle(t *testing.T) {
	repo, client := newForumRepoSQLite(t)
	ctx := context.Background()

	user1 := mustCreateTestUser(t, ctx, client, "author1@example.com")
	user2 := mustCreateTestUser(t, ctx, client, "author2@example.com")

	// 1. Create root post
	post := &service.ForumPost{
		UserID:       user1.ID,
		Content:      "General forum announcement",
		Status:       service.ForumPostStatusActive,
		LikeCount:    0,
		DislikeCount: 0,
		ReplyCount:   0,
	}
	err := repo.CreatePost(ctx, post)
	require.NoError(t, err)
	assert.Greater(t, post.ID, int64(0))

	// 2. GetByID
	fetched, err := repo.GetByID(ctx, post.ID)
	require.NoError(t, err)
	assert.Equal(t, "General forum announcement", fetched.Content)
	assert.Equal(t, "a***1@example.com", fetched.UserEmailMasked)

	// 3. Create reply
	reply := &service.ForumPost{
		UserID:       user2.ID,
		ParentID:     &post.ID,
		Content:      "Interesting update!",
		Status:       service.ForumPostStatusActive,
		LikeCount:    0,
		DislikeCount: 0,
		ReplyCount:   0,
	}
	err = repo.CreatePost(ctx, reply)
	require.NoError(t, err)
	_ = repo.IncrementReplyCount(ctx, post.ID)

	// 4. List root posts
	posts, pagPosts, err := repo.ListPosts(ctx, service.ForumPostListQuery{
		Page:     1,
		PageSize: 10,
		UserID:   user2.ID,
	})
	require.NoError(t, err)
	assert.Len(t, posts, 1)
	assert.Equal(t, int64(1), pagPosts.Total)

	// 5. List replies
	replies, pagReplies, err := repo.ListReplies(ctx, post.ID, 1, 10, user2.ID)
	require.NoError(t, err)
	assert.Len(t, replies, 1)
	assert.Equal(t, int64(1), pagReplies.Total)
	assert.Equal(t, "Interesting update!", replies[0].Content)

	// 6. Voting (Like -> Dislike -> Cancel)
	voteRes, err := repo.SetVote(ctx, post.ID, user2.ID, service.ForumVoteTypeLike)
	require.NoError(t, err)
	assert.Equal(t, 1, voteRes.LikeCount)
	assert.Equal(t, 0, voteRes.DislikeCount)
	assert.Equal(t, "like", voteRes.UserVote)

	// Dislike (switch)
	voteRes, err = repo.SetVote(ctx, post.ID, user2.ID, service.ForumVoteTypeDislike)
	require.NoError(t, err)
	assert.Equal(t, 0, voteRes.LikeCount)
	assert.Equal(t, 1, voteRes.DislikeCount)
	assert.Equal(t, "dislike", voteRes.UserVote)

	// None (cancel)
	voteRes, err = repo.SetVote(ctx, post.ID, user2.ID, service.ForumVoteTypeNone)
	require.NoError(t, err)
	assert.Equal(t, 0, voteRes.LikeCount)
	assert.Equal(t, 0, voteRes.DislikeCount)
	assert.Equal(t, "none", voteRes.UserVote)

	// 7. Admin operations
	err = repo.AdminSetStatus(ctx, post.ID, service.ForumPostStatusHidden)
	require.NoError(t, err)

	adminPosts, pagAdmin, err := repo.AdminListPosts(ctx, service.ForumPostStatusHidden, 1, 10)
	require.NoError(t, err)
	assert.Len(t, adminPosts, 1)
	assert.Equal(t, int64(1), pagAdmin.Total)
}
