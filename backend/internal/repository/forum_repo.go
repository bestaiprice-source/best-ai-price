package repository

import (
	"context"
	"math"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/forumpost"
	"github.com/Wei-Shaw/sub2api/ent/forumpostvote"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type forumRepository struct {
	client *dbent.Client
}

func NewForumRepository(client *dbent.Client) service.ForumRepository {
	return &forumRepository{client: client}
}

func (r *forumRepository) CreatePost(ctx context.Context, p *service.ForumPost) error {
	client := clientFromContext(ctx, r.client)
	builder := client.ForumPost.Create().
		SetUserID(p.UserID).
		SetContent(p.Content).
		SetStatus(p.Status).
		SetLikeCount(p.LikeCount).
		SetDislikeCount(p.DislikeCount).
		SetReplyCount(p.ReplyCount)

	if p.ParentID != nil && *p.ParentID > 0 {
		builder.SetParentID(*p.ParentID)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	p.ID = created.ID
	p.CreatedAt = created.CreatedAt
	p.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *forumRepository) GetByID(ctx context.Context, id int64) (*service.ForumPost, error) {
	entPost, err := r.client.ForumPost.Query().
		Where(forumpost.IDEQ(id)).
		WithUser().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrForumPostNotFound, nil)
	}

	return r.mapEntityToService(entPost, ""), nil
}

func (r *forumRepository) ListPosts(ctx context.Context, query service.ForumPostListQuery) ([]service.ForumPost, *pagination.PaginationResult, error) {
	q := r.client.ForumPost.Query().
		Where(
			forumpost.ParentIDIsNil(),
			forumpost.StatusEQ(service.ForumPostStatusActive),
		)

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	switch query.Sort {
	case "top_liked":
		q = q.Order(dbent.Desc(forumpost.FieldLikeCount), dbent.Desc(forumpost.FieldCreatedAt))
	case "most_replies":
		q = q.Order(dbent.Desc(forumpost.FieldReplyCount), dbent.Desc(forumpost.FieldCreatedAt))
	default:
		q = q.Order(dbent.Desc(forumpost.FieldCreatedAt))
	}

	offset := (query.Page - 1) * query.PageSize
	entities, err := q.Limit(query.PageSize).
		Offset(offset).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	postIDs := make([]int64, len(entities))
	for i, e := range entities {
		postIDs[i] = e.ID
	}

	var userVotes map[int64]string
	if query.UserID > 0 && len(postIDs) > 0 {
		userVotes, _ = r.GetUserVotes(ctx, query.UserID, postIDs)
	}

	posts := make([]service.ForumPost, len(entities))
	for i, e := range entities {
		vote := ""
		if userVotes != nil {
			vote = userVotes[e.ID]
		}
		posts[i] = *r.mapEntityToService(e, vote)
	}

	pagResult := &pagination.PaginationResult{
		Total:    int64(total),
		Page:     query.Page,
		PageSize: query.PageSize,
		Pages:    int(math.Ceil(float64(total) / float64(query.PageSize))),
	}

	return posts, pagResult, nil
}

func (r *forumRepository) ListReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]service.ForumPost, *pagination.PaginationResult, error) {
	q := r.client.ForumPost.Query().
		Where(
			forumpost.ParentIDEQ(parentID),
			forumpost.StatusEQ(service.ForumPostStatusActive),
		)

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	offset := (page - 1) * pageSize
	entities, err := q.Order(dbent.Asc(forumpost.FieldCreatedAt)).
		Limit(pageSize).
		Offset(offset).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	postIDs := make([]int64, len(entities))
	for i, e := range entities {
		postIDs[i] = e.ID
	}

	var userVotes map[int64]string
	if currentUserID > 0 && len(postIDs) > 0 {
		userVotes, _ = r.GetUserVotes(ctx, currentUserID, postIDs)
	}

	replies := make([]service.ForumPost, len(entities))
	for i, e := range entities {
		vote := ""
		if userVotes != nil {
			vote = userVotes[e.ID]
		}
		replies[i] = *r.mapEntityToService(e, vote)
	}

	pagResult := &pagination.PaginationResult{
		Total:    int64(total),
		Page:     page,
		PageSize: pageSize,
		Pages:    int(math.Ceil(float64(total) / float64(pageSize))),
	}

	return replies, pagResult, nil
}

func (r *forumRepository) IncrementReplyCount(ctx context.Context, postID int64) error {
	client := clientFromContext(ctx, r.client)
	return client.ForumPost.UpdateOneID(postID).
		AddReplyCount(1).
		Exec(ctx)
}

func (r *forumRepository) SetVote(ctx context.Context, postID, userID int64, voteType string) (*service.ForumVoteResult, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	existingVote, err := tx.ForumPostVote.Query().
		Where(
			forumpostvote.PostIDEQ(postID),
			forumpostvote.UserIDEQ(userID),
		).
		First(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, err
	}

	post, err := tx.ForumPost.Query().
		Where(forumpost.IDEQ(postID)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrForumPostNotFound, nil)
	}

	likeDelta := 0
	dislikeDelta := 0
	finalVote := voteType

	if existingVote != nil {
		if voteType == service.ForumVoteTypeNone || voteType == existingVote.VoteType {
			// Remove vote
			if existingVote.VoteType == service.ForumVoteTypeLike {
				likeDelta = -1
			} else if existingVote.VoteType == service.ForumVoteTypeDislike {
				dislikeDelta = -1
			}
			if err := tx.ForumPostVote.DeleteOne(existingVote).Exec(ctx); err != nil {
				return nil, err
			}
			finalVote = service.ForumVoteTypeNone
		} else {
			// Change vote from like -> dislike or dislike -> like
			if existingVote.VoteType == service.ForumVoteTypeLike && voteType == service.ForumVoteTypeDislike {
				likeDelta = -1
				dislikeDelta = 1
			} else if existingVote.VoteType == service.ForumVoteTypeDislike && voteType == service.ForumVoteTypeLike {
				likeDelta = 1
				dislikeDelta = -1
			}
			if err := tx.ForumPostVote.UpdateOne(existingVote).SetVoteType(voteType).Exec(ctx); err != nil {
				return nil, err
			}
		}
	} else {
		// New vote
		if voteType != service.ForumVoteTypeNone {
			if voteType == service.ForumVoteTypeLike {
				likeDelta = 1
			} else if voteType == service.ForumVoteTypeDislike {
				dislikeDelta = 1
			}
			if err := tx.ForumPostVote.Create().
				SetPostID(postID).
				SetUserID(userID).
				SetVoteType(voteType).
				Exec(ctx); err != nil {
				return nil, err
			}
		}
	}

	// Update counts
	newLikes := maxInt(0, post.LikeCount+likeDelta)
	newDislikes := maxInt(0, post.DislikeCount+dislikeDelta)

	if err := tx.ForumPost.UpdateOneID(postID).
		SetLikeCount(newLikes).
		SetDislikeCount(newDislikes).
		Exec(ctx); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &service.ForumVoteResult{
		PostID:       postID,
		LikeCount:    newLikes,
		DislikeCount: newDislikes,
		UserVote:     finalVote,
	}, nil
}

func (r *forumRepository) GetUserVotes(ctx context.Context, userID int64, postIDs []int64) (map[int64]string, error) {
	if len(postIDs) == 0 {
		return make(map[int64]string), nil
	}

	votes, err := r.client.ForumPostVote.Query().
		Where(
			forumpostvote.UserIDEQ(userID),
			forumpostvote.PostIDIn(postIDs...),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]string, len(votes))
	for _, v := range votes {
		result[v.PostID] = v.VoteType
	}
	return result, nil
}

func (r *forumRepository) AdminListPosts(ctx context.Context, status string, page, pageSize int) ([]service.ForumPost, *pagination.PaginationResult, error) {
	q := r.client.ForumPost.Query()

	if status != "" {
		q = q.Where(forumpost.StatusEQ(status))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	offset := (page - 1) * pageSize
	entities, err := q.Order(dbent.Desc(forumpost.FieldCreatedAt)).
		Limit(pageSize).
		Offset(offset).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	posts := make([]service.ForumPost, len(entities))
	for i, e := range entities {
		posts[i] = *r.mapEntityToService(e, "")
	}

	pagResult := &pagination.PaginationResult{
		Total:    int64(total),
		Page:     page,
		PageSize: pageSize,
		Pages:    int(math.Ceil(float64(total) / float64(pageSize))),
	}

	return posts, pagResult, nil
}

func (r *forumRepository) AdminSetStatus(ctx context.Context, id int64, status string) error {
	client := clientFromContext(ctx, r.client)
	return client.ForumPost.UpdateOneID(id).
		SetStatus(status).
		Exec(ctx)
}

func (r *forumRepository) mapEntityToService(e *dbent.ForumPost, userVote string) *service.ForumPost {
	if e == nil {
		return nil
	}

	emailMasked := ""
	if e.Edges.User != nil {
		emailMasked = service.MaskEmail(e.Edges.User.Email)
	}

	res := &service.ForumPost{
		ID:              e.ID,
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
	return res
}
