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

	// 1. Create topic
	topic := &service.ForumTopic{
		UserID:       user1.ID,
		Title:        "General Announcement",
		Content:      "Welcome to the forum!",
		Status:       service.ForumTopicStatusActive,
		LikeCount:    0,
		DislikeCount: 0,
		CommentCount: 0,
	}
	err := repo.CreateTopic(ctx, topic)
	require.NoError(t, err)
	assert.Greater(t, topic.ID, int64(0))

	topic2 := &service.ForumTopic{
		UserID:       user2.ID,
		Title:        "Another Topic Searchable",
		Content:      "Another discussion",
		Status:       service.ForumTopicStatusActive,
		LikeCount:    5,
		DislikeCount: 0,
		CommentCount: 2,
	}
	err = repo.CreateTopic(ctx, topic2)
	require.NoError(t, err)

	// 2. GetTopicByID
	fetchedTopic, err := repo.GetTopicByID(ctx, topic.ID)
	require.NoError(t, err)
	assert.Equal(t, "General Announcement", fetchedTopic.Title)
	assert.Equal(t, "Welcome to the forum!", fetchedTopic.Content)
	assert.Equal(t, "a***1@example.com", fetchedTopic.UserEmailMasked)

	_, err = repo.GetTopicByID(ctx, 99999)
	assert.ErrorIs(t, err, service.ErrForumTopicNotFound)

	// 3. ListTopics with various sorts & search
	topics, pagTopics, err := repo.ListTopics(ctx, service.ForumTopicListQuery{
		Page:     1,
		PageSize: 10,
		UserID:   user2.ID,
	})
	require.NoError(t, err)
	assert.Len(t, topics, 2)
	assert.Equal(t, int64(2), pagTopics.Total)

	// Sort top_liked
	topicsLiked, _, err := repo.ListTopics(ctx, service.ForumTopicListQuery{
		Sort:     "top_liked",
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, topic2.ID, topicsLiked[0].ID)

	// Sort most_comments
	topicsComments, _, err := repo.ListTopics(ctx, service.ForumTopicListQuery{
		Sort:     "most_comments",
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, topic2.ID, topicsComments[0].ID)

	// Search
	topicsSearch, _, err := repo.ListTopics(ctx, service.ForumTopicListQuery{
		Search:   "searchable",
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Len(t, topicsSearch, 1)
	assert.Equal(t, topic2.ID, topicsSearch[0].ID)

	// 4. Topic Voting (Like -> Dislike -> Cancel)
	voteRes, err := repo.SetTopicVote(ctx, topic.ID, user2.ID, service.ForumVoteTypeLike)
	require.NoError(t, err)
	assert.Equal(t, 1, voteRes.LikeCount)
	assert.Equal(t, 0, voteRes.DislikeCount)
	assert.Equal(t, "like", voteRes.UserVote)

	// Switch from like to dislike
	voteRes, err = repo.SetTopicVote(ctx, topic.ID, user2.ID, service.ForumVoteTypeDislike)
	require.NoError(t, err)
	assert.Equal(t, 0, voteRes.LikeCount)
	assert.Equal(t, 1, voteRes.DislikeCount)
	assert.Equal(t, "dislike", voteRes.UserVote)

	// Switch from dislike to like
	voteRes, err = repo.SetTopicVote(ctx, topic.ID, user2.ID, service.ForumVoteTypeLike)
	require.NoError(t, err)
	assert.Equal(t, 1, voteRes.LikeCount)
	assert.Equal(t, 0, voteRes.DislikeCount)
	assert.Equal(t, "like", voteRes.UserVote)

	// None (cancel)
	voteRes, err = repo.SetTopicVote(ctx, topic.ID, user2.ID, service.ForumVoteTypeNone)
	require.NoError(t, err)
	assert.Equal(t, 0, voteRes.LikeCount)
	assert.Equal(t, 0, voteRes.DislikeCount)
	assert.Equal(t, "none", voteRes.UserVote)

	// 5. Create Root Comment
	comment := &service.ForumComment{
		TopicID:      topic.ID,
		UserID:       user2.ID,
		Content:      "First comment here!",
		Status:       service.ForumCommentStatusActive,
		LikeCount:    0,
		DislikeCount: 0,
		ReplyCount:   0,
	}
	err = repo.CreateComment(ctx, comment)
	require.NoError(t, err)
	assert.Greater(t, comment.ID, int64(0))
	_ = repo.IncrementTopicCommentCount(ctx, topic.ID)

	// 6. GetCommentByID
	fetchedComment, err := repo.GetCommentByID(ctx, comment.ID)
	require.NoError(t, err)
	assert.Equal(t, "First comment here!", fetchedComment.Content)
	assert.Equal(t, "a***2@example.com", fetchedComment.UserEmailMasked)

	_, err = repo.GetCommentByID(ctx, 99999)
	assert.ErrorIs(t, err, service.ErrForumCommentNotFound)

	// 7. Create Reply Comment
	reply := &service.ForumComment{
		TopicID:      topic.ID,
		UserID:       user1.ID,
		ParentID:     &comment.ID,
		Content:      "Reply to first comment",
		Status:       service.ForumCommentStatusActive,
		LikeCount:    0,
		DislikeCount: 0,
		ReplyCount:   0,
	}
	err = repo.CreateComment(ctx, reply)
	require.NoError(t, err)
	_ = repo.IncrementCommentReplyCount(ctx, comment.ID)

	// 8. List Comments by Topic (default, oldest, top_liked)
	comments, pagComments, err := repo.ListCommentsByTopic(ctx, service.ForumCommentListQuery{
		TopicID:  topic.ID,
		Page:     1,
		PageSize: 10,
		UserID:   user1.ID,
	})
	require.NoError(t, err)
	assert.Len(t, comments, 1) // only root comments
	assert.Equal(t, int64(1), pagComments.Total)

	commentsOldest, _, err := repo.ListCommentsByTopic(ctx, service.ForumCommentListQuery{
		TopicID:  topic.ID,
		Sort:     "oldest",
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Len(t, commentsOldest, 1)

	commentsTopLiked, _, err := repo.ListCommentsByTopic(ctx, service.ForumCommentListQuery{
		TopicID:  topic.ID,
		Sort:     "top_liked",
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Len(t, commentsTopLiked, 1)

	// 9. List Comment Replies
	replies, pagReplies, err := repo.ListCommentReplies(ctx, comment.ID, 1, 10, user1.ID)
	require.NoError(t, err)
	assert.Len(t, replies, 1)
	assert.Equal(t, int64(1), pagReplies.Total)
	assert.Equal(t, "Reply to first comment", replies[0].Content)

	// 10. Comment Voting (like -> dislike -> like -> cancel)
	commentVoteRes, err := repo.SetCommentVote(ctx, comment.ID, user1.ID, service.ForumVoteTypeLike)
	require.NoError(t, err)
	assert.Equal(t, 1, commentVoteRes.LikeCount)
	assert.Equal(t, "like", commentVoteRes.UserVote)

	commentVoteRes, err = repo.SetCommentVote(ctx, comment.ID, user1.ID, service.ForumVoteTypeDislike)
	require.NoError(t, err)
	assert.Equal(t, 0, commentVoteRes.LikeCount)
	assert.Equal(t, 1, commentVoteRes.DislikeCount)
	assert.Equal(t, "dislike", commentVoteRes.UserVote)

	commentVoteRes, err = repo.SetCommentVote(ctx, comment.ID, user1.ID, service.ForumVoteTypeLike)
	require.NoError(t, err)
	assert.Equal(t, 1, commentVoteRes.LikeCount)
	assert.Equal(t, 0, commentVoteRes.DislikeCount)
	assert.Equal(t, "like", commentVoteRes.UserVote)

	commentVoteRes, err = repo.SetCommentVote(ctx, comment.ID, user1.ID, service.ForumVoteTypeNone)
	require.NoError(t, err)
	assert.Equal(t, 0, commentVoteRes.LikeCount)
	assert.Equal(t, 0, commentVoteRes.DislikeCount)
	assert.Equal(t, "none", commentVoteRes.UserVote)

	// 11. Admin operations
	err = repo.AdminSetTopicStatus(ctx, topic.ID, service.ForumTopicStatusHidden)
	require.NoError(t, err)

	adminTopics, pagAdminT, err := repo.AdminListTopics(ctx, service.ForumTopicStatusHidden, 1, 10)
	require.NoError(t, err)
	assert.Len(t, adminTopics, 1)
	assert.Equal(t, int64(1), pagAdminT.Total)

	err = repo.AdminSetCommentStatus(ctx, comment.ID, service.ForumCommentStatusHidden)
	require.NoError(t, err)

	adminComments, pagAdminC, err := repo.AdminListComments(ctx, service.ForumCommentStatusHidden, 1, 10)
	require.NoError(t, err)
	assert.Len(t, adminComments, 1)
	assert.Equal(t, int64(1), pagAdminC.Total)
}
