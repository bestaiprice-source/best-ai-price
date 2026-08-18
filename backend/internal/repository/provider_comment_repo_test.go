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

func newProviderCommentRepoSQLite(t *testing.T) (service.ProviderCommentRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:provider_comment_repo_test?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	repo := NewProviderCommentRepository(client)
	return repo, client
}

func mustCreateTestUser(t *testing.T, ctx context.Context, client *dbent.Client, email string) *dbent.User {
	t.Helper()
	u, err := client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	return u
}

func TestProviderCommentRepository_FullLifecycle(t *testing.T) {
	repo, client := newProviderCommentRepoSQLite(t)
	ctx := context.Background()

	user1 := mustCreateTestUser(t, ctx, client, "user1@example.com")
	user2 := mustCreateTestUser(t, ctx, client, "user2@example.com")

	// 1. Create root comment
	comment := &service.ProviderComment{
		Provider:     "yunwu",
		UserID:       user1.ID,
		Content:      "Yunwu is reliable and fast",
		Sentiment:    service.SentimentPositive,
		Status:       service.CommentStatusActive,
		LikeCount:    0,
		DislikeCount: 0,
		ReplyCount:   0,
	}
	err := repo.CreateComment(ctx, comment)
	require.NoError(t, err)
	assert.Greater(t, comment.ID, int64(0))

	// 2. GetByID
	fetched, err := repo.GetByID(ctx, comment.ID)
	require.NoError(t, err)
	assert.Equal(t, "yunwu", fetched.Provider)
	assert.Equal(t, "u***1@example.com", fetched.UserEmailMasked)

	// 3. Create reply
	reply := &service.ProviderComment{
		Provider:     "yunwu",
		UserID:       user2.ID,
		ParentID:     &comment.ID,
		Content:      "Totally agree!",
		Sentiment:    service.SentimentNeutral,
		Status:       service.CommentStatusActive,
		LikeCount:    0,
		DislikeCount: 0,
		ReplyCount:   0,
	}
	err = repo.CreateComment(ctx, reply)
	require.NoError(t, err)
	_ = repo.IncrementReplyCount(ctx, comment.ID)

	// 4. List replies
	replies, pagReplies, err := repo.ListReplies(ctx, comment.ID, 1, 10, user2.ID)
	require.NoError(t, err)
	assert.Len(t, replies, 1)
	assert.Equal(t, int64(1), pagReplies.Total)
	assert.Equal(t, "Totally agree!", replies[0].Content)

	// 5. Voting (Like -> Dislike -> None)
	voteRes, err := repo.SetVote(ctx, comment.ID, user2.ID, service.VoteTypeLike)
	require.NoError(t, err)
	assert.Equal(t, 1, voteRes.LikeCount)
	assert.Equal(t, 0, voteRes.DislikeCount)
	assert.Equal(t, service.VoteTypeLike, voteRes.UserVote)

	voteRes, err = repo.SetVote(ctx, comment.ID, user2.ID, service.VoteTypeDislike)
	require.NoError(t, err)
	assert.Equal(t, 0, voteRes.LikeCount)
	assert.Equal(t, 1, voteRes.DislikeCount)
	assert.Equal(t, service.VoteTypeDislike, voteRes.UserVote)

	voteRes, err = repo.SetVote(ctx, comment.ID, user2.ID, service.VoteTypeNone)
	require.NoError(t, err)
	assert.Equal(t, 0, voteRes.LikeCount)
	assert.Equal(t, 0, voteRes.DislikeCount)
	assert.Equal(t, service.VoteTypeNone, voteRes.UserVote)

	// 6. Summary stats
	summary, err := repo.GetProviderSummary(ctx, "yunwu")
	require.NoError(t, err)
	assert.Equal(t, 1, summary.TotalComments)
	assert.Equal(t, 1, summary.PositiveCount)
	assert.Equal(t, 100.0, summary.PositiveRatio)

	// 7. ListProviderComments
	list, pagList, err := repo.ListProviderComments(ctx, service.CommentListQuery{
		Provider: "yunwu",
		Page:     1,
		PageSize: 10,
		UserID:   user2.ID,
	})
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, int64(1), pagList.Total)
	assert.Equal(t, 1, list[0].ReplyCount)

	// 8. Admin operations
	err = repo.AdminSetStatus(ctx, comment.ID, service.CommentStatusHidden)
	require.NoError(t, err)

	adminList, _, err := repo.AdminListComments(ctx, "yunwu", service.CommentStatusHidden, "", 1, 10)
	require.NoError(t, err)
	assert.Len(t, adminList, 1)
}
