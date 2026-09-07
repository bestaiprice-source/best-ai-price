package repository

import (
	"context"
	"math"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/forumcomment"
	"github.com/Wei-Shaw/sub2api/ent/forumcommentvote"
	"github.com/Wei-Shaw/sub2api/ent/forumtopic"
	"github.com/Wei-Shaw/sub2api/ent/forumtopicvote"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type forumRepository struct {
	client *dbent.Client
}

func NewForumRepository(client *dbent.Client) service.ForumRepository {
	return &forumRepository{client: client}
}

// -----------------------------------------------------------------------------
// Topics
// -----------------------------------------------------------------------------

func (r *forumRepository) CreateTopic(ctx context.Context, t *service.ForumTopic) error {
	client := clientFromContext(ctx, r.client)
	builder := client.ForumTopic.Create().
		SetUserID(t.UserID).
		SetTitle(t.Title).
		SetContent(t.Content).
		SetStatus(t.Status).
		SetLikeCount(t.LikeCount).
		SetDislikeCount(t.DislikeCount).
		SetCommentCount(t.CommentCount)

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	t.ID = created.ID
	t.CreatedAt = created.CreatedAt
	t.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *forumRepository) GetTopicByID(ctx context.Context, id int64) (*service.ForumTopic, error) {
	entTopic, err := r.client.ForumTopic.Query().
		Where(forumtopic.IDEQ(id)).
		WithUser().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrForumTopicNotFound, nil)
	}

	return r.mapTopicEntityToService(entTopic, ""), nil
}

func (r *forumRepository) ListTopics(ctx context.Context, query service.ForumTopicListQuery) ([]service.ForumTopic, *pagination.PaginationResult, error) {
	q := r.client.ForumTopic.Query().
		Where(forumtopic.StatusEQ(service.ForumTopicStatusActive))

	if query.Search != "" {
		q = q.Where(forumtopic.TitleContainsFold(query.Search))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	switch query.Sort {
	case "top_liked":
		q = q.Order(dbent.Desc(forumtopic.FieldLikeCount), dbent.Desc(forumtopic.FieldCreatedAt))
	case "most_comments":
		q = q.Order(dbent.Desc(forumtopic.FieldCommentCount), dbent.Desc(forumtopic.FieldCreatedAt))
	default:
		q = q.Order(dbent.Desc(forumtopic.FieldCreatedAt))
	}

	offset := (query.Page - 1) * query.PageSize
	entities, err := q.Limit(query.PageSize).
		Offset(offset).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	topicIDs := make([]int64, len(entities))
	for i, e := range entities {
		topicIDs[i] = e.ID
	}

	var userVotes map[int64]string
	if query.UserID > 0 && len(topicIDs) > 0 {
		userVotes, _ = r.GetTopicUserVotes(ctx, query.UserID, topicIDs)
	}

	topics := make([]service.ForumTopic, len(entities))
	for i, e := range entities {
		vote := ""
		if userVotes != nil {
			vote = userVotes[e.ID]
		}
		topics[i] = *r.mapTopicEntityToService(e, vote)
	}

	pagResult := &pagination.PaginationResult{
		Total:    int64(total),
		Page:     query.Page,
		PageSize: query.PageSize,
		Pages:    int(math.Ceil(float64(total) / float64(query.PageSize))),
	}

	return topics, pagResult, nil
}

func (r *forumRepository) SetTopicVote(ctx context.Context, topicID, userID int64, voteType string) (*service.ForumVoteResult, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	existingVote, err := tx.ForumTopicVote.Query().
		Where(
			forumtopicvote.TopicIDEQ(topicID),
			forumtopicvote.UserIDEQ(userID),
		).
		First(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, err
	}

	topic, err := tx.ForumTopic.Query().
		Where(forumtopic.IDEQ(topicID)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrForumTopicNotFound, nil)
	}

	likeDelta := 0
	dislikeDelta := 0
	finalVote := voteType

	if existingVote != nil {
		if voteType == service.ForumVoteTypeNone || voteType == existingVote.VoteType {
			if existingVote.VoteType == service.ForumVoteTypeLike {
				likeDelta = -1
			} else if existingVote.VoteType == service.ForumVoteTypeDislike {
				dislikeDelta = -1
			}
			if err := tx.ForumTopicVote.DeleteOne(existingVote).Exec(ctx); err != nil {
				return nil, err
			}
			finalVote = service.ForumVoteTypeNone
		} else {
			if existingVote.VoteType == service.ForumVoteTypeLike && voteType == service.ForumVoteTypeDislike {
				likeDelta = -1
				dislikeDelta = 1
			} else if existingVote.VoteType == service.ForumVoteTypeDislike && voteType == service.ForumVoteTypeLike {
				likeDelta = 1
				dislikeDelta = -1
			}
			if err := tx.ForumTopicVote.UpdateOne(existingVote).SetVoteType(voteType).Exec(ctx); err != nil {
				return nil, err
			}
		}
	} else {
		if voteType != service.ForumVoteTypeNone {
			if voteType == service.ForumVoteTypeLike {
				likeDelta = 1
			} else if voteType == service.ForumVoteTypeDislike {
				dislikeDelta = 1
			}
			if err := tx.ForumTopicVote.Create().
				SetTopicID(topicID).
				SetUserID(userID).
				SetVoteType(voteType).
				Exec(ctx); err != nil {
				return nil, err
			}
		}
	}

	newLikes := maxInt(0, topic.LikeCount+likeDelta)
	newDislikes := maxInt(0, topic.DislikeCount+dislikeDelta)

	if err := tx.ForumTopic.UpdateOneID(topicID).
		SetLikeCount(newLikes).
		SetDislikeCount(newDislikes).
		Exec(ctx); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &service.ForumVoteResult{
		ID:           topicID,
		LikeCount:    newLikes,
		DislikeCount: newDislikes,
		UserVote:     finalVote,
	}, nil
}

func (r *forumRepository) GetTopicUserVotes(ctx context.Context, userID int64, topicIDs []int64) (map[int64]string, error) {
	if len(topicIDs) == 0 {
		return make(map[int64]string), nil
	}

	votes, err := r.client.ForumTopicVote.Query().
		Where(
			forumtopicvote.UserIDEQ(userID),
			forumtopicvote.TopicIDIn(topicIDs...),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]string, len(votes))
	for _, v := range votes {
		result[v.TopicID] = v.VoteType
	}
	return result, nil
}

func (r *forumRepository) AdminListTopics(ctx context.Context, status string, page, pageSize int) ([]service.ForumTopic, *pagination.PaginationResult, error) {
	q := r.client.ForumTopic.Query()

	if status != "" {
		q = q.Where(forumtopic.StatusEQ(status))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	offset := (page - 1) * pageSize
	entities, err := q.Order(dbent.Desc(forumtopic.FieldCreatedAt)).
		Limit(pageSize).
		Offset(offset).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	topics := make([]service.ForumTopic, len(entities))
	for i, e := range entities {
		topics[i] = *r.mapTopicEntityToService(e, "")
	}

	pagResult := &pagination.PaginationResult{
		Total:    int64(total),
		Page:     page,
		PageSize: pageSize,
		Pages:    int(math.Ceil(float64(total) / float64(pageSize))),
	}

	return topics, pagResult, nil
}

func (r *forumRepository) AdminSetTopicStatus(ctx context.Context, id int64, status string) error {
	client := clientFromContext(ctx, r.client)
	return client.ForumTopic.UpdateOneID(id).
		SetStatus(status).
		Exec(ctx)
}

// -----------------------------------------------------------------------------
// Comments
// -----------------------------------------------------------------------------

func (r *forumRepository) CreateComment(ctx context.Context, c *service.ForumComment) error {
	client := clientFromContext(ctx, r.client)
	builder := client.ForumComment.Create().
		SetTopicID(c.TopicID).
		SetUserID(c.UserID).
		SetContent(c.Content).
		SetStatus(c.Status).
		SetLikeCount(c.LikeCount).
		SetDislikeCount(c.DislikeCount).
		SetReplyCount(c.ReplyCount)

	if c.ParentID != nil && *c.ParentID > 0 {
		builder.SetParentID(*c.ParentID)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	c.ID = created.ID
	c.CreatedAt = created.CreatedAt
	c.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *forumRepository) GetCommentByID(ctx context.Context, id int64) (*service.ForumComment, error) {
	entComment, err := r.client.ForumComment.Query().
		Where(forumcomment.IDEQ(id)).
		WithUser().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrForumCommentNotFound, nil)
	}

	return r.mapCommentEntityToService(entComment, ""), nil
}

func (r *forumRepository) ListCommentsByTopic(ctx context.Context, query service.ForumCommentListQuery) ([]service.ForumComment, *pagination.PaginationResult, error) {
	q := r.client.ForumComment.Query().
		Where(
			forumcomment.TopicIDEQ(query.TopicID),
			forumcomment.ParentIDIsNil(),
			forumcomment.StatusEQ(service.ForumCommentStatusActive),
		)

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	switch query.Sort {
	case "top_liked":
		q = q.Order(dbent.Desc(forumcomment.FieldLikeCount), dbent.Desc(forumcomment.FieldCreatedAt))
	case "oldest":
		q = q.Order(dbent.Asc(forumcomment.FieldCreatedAt))
	default:
		q = q.Order(dbent.Desc(forumcomment.FieldCreatedAt))
	}

	offset := (query.Page - 1) * query.PageSize
	entities, err := q.Limit(query.PageSize).
		Offset(offset).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	commentIDs := make([]int64, len(entities))
	for i, e := range entities {
		commentIDs[i] = e.ID
	}

	var userVotes map[int64]string
	if query.UserID > 0 && len(commentIDs) > 0 {
		userVotes, _ = r.GetCommentUserVotes(ctx, query.UserID, commentIDs)
	}

	comments := make([]service.ForumComment, len(entities))
	for i, e := range entities {
		vote := ""
		if userVotes != nil {
			vote = userVotes[e.ID]
		}
		comments[i] = *r.mapCommentEntityToService(e, vote)
	}

	pagResult := &pagination.PaginationResult{
		Total:    int64(total),
		Page:     query.Page,
		PageSize: query.PageSize,
		Pages:    int(math.Ceil(float64(total) / float64(query.PageSize))),
	}

	return comments, pagResult, nil
}

func (r *forumRepository) ListCommentReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]service.ForumComment, *pagination.PaginationResult, error) {
	q := r.client.ForumComment.Query().
		Where(
			forumcomment.ParentIDEQ(parentID),
			forumcomment.StatusEQ(service.ForumCommentStatusActive),
		)

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	offset := (page - 1) * pageSize
	entities, err := q.Order(dbent.Asc(forumcomment.FieldCreatedAt)).
		Limit(pageSize).
		Offset(offset).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	commentIDs := make([]int64, len(entities))
	for i, e := range entities {
		commentIDs[i] = e.ID
	}

	var userVotes map[int64]string
	if currentUserID > 0 && len(commentIDs) > 0 {
		userVotes, _ = r.GetCommentUserVotes(ctx, currentUserID, commentIDs)
	}

	replies := make([]service.ForumComment, len(entities))
	for i, e := range entities {
		vote := ""
		if userVotes != nil {
			vote = userVotes[e.ID]
		}
		replies[i] = *r.mapCommentEntityToService(e, vote)
	}

	pagResult := &pagination.PaginationResult{
		Total:    int64(total),
		Page:     page,
		PageSize: pageSize,
		Pages:    int(math.Ceil(float64(total) / float64(pageSize))),
	}

	return replies, pagResult, nil
}

func (r *forumRepository) IncrementTopicCommentCount(ctx context.Context, topicID int64) error {
	client := clientFromContext(ctx, r.client)
	return client.ForumTopic.UpdateOneID(topicID).
		AddCommentCount(1).
		Exec(ctx)
}

func (r *forumRepository) IncrementCommentReplyCount(ctx context.Context, commentID int64) error {
	client := clientFromContext(ctx, r.client)
	return client.ForumComment.UpdateOneID(commentID).
		AddReplyCount(1).
		Exec(ctx)
}

func (r *forumRepository) SetCommentVote(ctx context.Context, commentID, userID int64, voteType string) (*service.ForumVoteResult, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	existingVote, err := tx.ForumCommentVote.Query().
		Where(
			forumcommentvote.CommentIDEQ(commentID),
			forumcommentvote.UserIDEQ(userID),
		).
		First(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, err
	}

	comment, err := tx.ForumComment.Query().
		Where(forumcomment.IDEQ(commentID)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrForumCommentNotFound, nil)
	}

	likeDelta := 0
	dislikeDelta := 0
	finalVote := voteType

	if existingVote != nil {
		if voteType == service.ForumVoteTypeNone || voteType == existingVote.VoteType {
			if existingVote.VoteType == service.ForumVoteTypeLike {
				likeDelta = -1
			} else if existingVote.VoteType == service.ForumVoteTypeDislike {
				dislikeDelta = -1
			}
			if err := tx.ForumCommentVote.DeleteOne(existingVote).Exec(ctx); err != nil {
				return nil, err
			}
			finalVote = service.ForumVoteTypeNone
		} else {
			if existingVote.VoteType == service.ForumVoteTypeLike && voteType == service.ForumVoteTypeDislike {
				likeDelta = -1
				dislikeDelta = 1
			} else if existingVote.VoteType == service.ForumVoteTypeDislike && voteType == service.ForumVoteTypeLike {
				likeDelta = 1
				dislikeDelta = -1
			}
			if err := tx.ForumCommentVote.UpdateOne(existingVote).SetVoteType(voteType).Exec(ctx); err != nil {
				return nil, err
			}
		}
	} else {
		if voteType != service.ForumVoteTypeNone {
			if voteType == service.ForumVoteTypeLike {
				likeDelta = 1
			} else if voteType == service.ForumVoteTypeDislike {
				dislikeDelta = 1
			}
			if err := tx.ForumCommentVote.Create().
				SetCommentID(commentID).
				SetUserID(userID).
				SetVoteType(voteType).
				Exec(ctx); err != nil {
				return nil, err
			}
		}
	}

	newLikes := maxInt(0, comment.LikeCount+likeDelta)
	newDislikes := maxInt(0, comment.DislikeCount+dislikeDelta)

	if err := tx.ForumComment.UpdateOneID(commentID).
		SetLikeCount(newLikes).
		SetDislikeCount(newDislikes).
		Exec(ctx); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &service.ForumVoteResult{
		ID:           commentID,
		LikeCount:    newLikes,
		DislikeCount: newDislikes,
		UserVote:     finalVote,
	}, nil
}

func (r *forumRepository) GetCommentUserVotes(ctx context.Context, userID int64, commentIDs []int64) (map[int64]string, error) {
	if len(commentIDs) == 0 {
		return make(map[int64]string), nil
	}

	votes, err := r.client.ForumCommentVote.Query().
		Where(
			forumcommentvote.UserIDEQ(userID),
			forumcommentvote.CommentIDIn(commentIDs...),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]string, len(votes))
	for _, v := range votes {
		result[v.CommentID] = v.VoteType
	}
	return result, nil
}

func (r *forumRepository) AdminListComments(ctx context.Context, status string, page, pageSize int) ([]service.ForumComment, *pagination.PaginationResult, error) {
	q := r.client.ForumComment.Query()

	if status != "" {
		q = q.Where(forumcomment.StatusEQ(status))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	offset := (page - 1) * pageSize
	entities, err := q.Order(dbent.Desc(forumcomment.FieldCreatedAt)).
		Limit(pageSize).
		Offset(offset).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	comments := make([]service.ForumComment, len(entities))
	for i, e := range entities {
		comments[i] = *r.mapCommentEntityToService(e, "")
	}

	pagResult := &pagination.PaginationResult{
		Total:    int64(total),
		Page:     page,
		PageSize: pageSize,
		Pages:    int(math.Ceil(float64(total) / float64(pageSize))),
	}

	return comments, pagResult, nil
}

func (r *forumRepository) AdminSetCommentStatus(ctx context.Context, id int64, status string) error {
	client := clientFromContext(ctx, r.client)
	return client.ForumComment.UpdateOneID(id).
		SetStatus(status).
		Exec(ctx)
}

// -----------------------------------------------------------------------------
// Mapping Helpers
// -----------------------------------------------------------------------------

func (r *forumRepository) mapTopicEntityToService(e *dbent.ForumTopic, userVote string) *service.ForumTopic {
	if e == nil {
		return nil
	}

	emailMasked := ""
	if e.Edges.User != nil {
		emailMasked = service.MaskEmail(e.Edges.User.Email)
	}

	return &service.ForumTopic{
		ID:              e.ID,
		UserID:          e.UserID,
		UserEmailMasked: emailMasked,
		Title:           e.Title,
		Content:         e.Content,
		Status:          e.Status,
		LikeCount:       e.LikeCount,
		DislikeCount:    e.DislikeCount,
		CommentCount:    e.CommentCount,
		UserVote:        userVote,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
	}
}

func (r *forumRepository) mapCommentEntityToService(e *dbent.ForumComment, userVote string) *service.ForumComment {
	if e == nil {
		return nil
	}

	emailMasked := ""
	if e.Edges.User != nil {
		emailMasked = service.MaskEmail(e.Edges.User.Email)
	}

	return &service.ForumComment{
		ID:              e.ID,
		TopicID:         e.TopicID,
		UserID:          e.UserID,
		UserEmailMasked: emailMasked,
		ParentID:        e.ParentID,
		Content:         e.Content,
		Status:          e.Status,
		LikeCount:       e.LikeCount,
		DislikeCount:    e.DislikeCount,
		ReplyCount:      e.ReplyCount,
		UserVote:        userVote,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
	}
}
