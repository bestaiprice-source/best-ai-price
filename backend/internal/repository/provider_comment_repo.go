package repository

import (
	"context"
	"math"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/providercomment"
	"github.com/Wei-Shaw/sub2api/ent/providercommentvote"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type providerCommentRepository struct {
	client *dbent.Client
}

func NewProviderCommentRepository(client *dbent.Client) service.ProviderCommentRepository {
	return &providerCommentRepository{client: client}
}

func (r *providerCommentRepository) CreateComment(ctx context.Context, c *service.ProviderComment) error {
	client := clientFromContext(ctx, r.client)
	builder := client.ProviderComment.Create().
		SetProvider(c.Provider).
		SetUserID(c.UserID).
		SetContent(c.Content).
		SetSentiment(c.Sentiment).
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

func (r *providerCommentRepository) GetByID(ctx context.Context, id int64) (*service.ProviderComment, error) {
	entComment, err := r.client.ProviderComment.Query().
		Where(providercomment.IDEQ(id)).
		WithUser().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrCommentNotFound, nil)
	}

	return r.mapEntityToService(entComment, ""), nil
}

func (r *providerCommentRepository) ListProviderComments(ctx context.Context, query service.CommentListQuery) ([]service.ProviderComment, *pagination.PaginationResult, error) {
	q := r.client.ProviderComment.Query().
		Where(
			providercomment.ProviderEQ(query.Provider),
			providercomment.ParentIDIsNil(),
			providercomment.StatusEQ(service.CommentStatusActive),
		)

	if query.Sentiment == service.SentimentPositive || query.Sentiment == service.SentimentNegative {
		q = q.Where(providercomment.SentimentEQ(query.Sentiment))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	switch query.Sort {
	case "top_liked":
		q = q.Order(dbent.Desc(providercomment.FieldLikeCount), dbent.Desc(providercomment.FieldCreatedAt))
	case "most_replies":
		q = q.Order(dbent.Desc(providercomment.FieldReplyCount), dbent.Desc(providercomment.FieldCreatedAt))
	default:
		q = q.Order(dbent.Desc(providercomment.FieldCreatedAt))
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
		userVotes, _ = r.GetUserVotes(ctx, query.UserID, commentIDs)
	}

	comments := make([]service.ProviderComment, len(entities))
	for i, e := range entities {
		vote := ""
		if userVotes != nil {
			vote = userVotes[e.ID]
		}
		comments[i] = *r.mapEntityToService(e, vote)
	}

	pagResult := &pagination.PaginationResult{
		Total:    int64(total),
		Page:     query.Page,
		PageSize: query.PageSize,
		Pages:    int(math.Ceil(float64(total) / float64(query.PageSize))),
	}

	return comments, pagResult, nil
}

func (r *providerCommentRepository) ListReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]service.ProviderComment, *pagination.PaginationResult, error) {
	q := r.client.ProviderComment.Query().
		Where(
			providercomment.ParentIDEQ(parentID),
			providercomment.StatusEQ(service.CommentStatusActive),
		)

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	offset := (page - 1) * pageSize
	entities, err := q.Order(dbent.Asc(providercomment.FieldCreatedAt)).
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
		userVotes, _ = r.GetUserVotes(ctx, currentUserID, commentIDs)
	}

	replies := make([]service.ProviderComment, len(entities))
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

func (r *providerCommentRepository) GetProviderSummary(ctx context.Context, provider string) (*service.ProviderCommentSummary, error) {
	baseQuery := r.client.ProviderComment.Query().
		Where(
			providercomment.ProviderEQ(provider),
			providercomment.ParentIDIsNil(),
			providercomment.StatusEQ(service.CommentStatusActive),
		)

	total, err := baseQuery.Count(ctx)
	if err != nil {
		return nil, err
	}

	posCount, err := r.client.ProviderComment.Query().
		Where(
			providercomment.ProviderEQ(provider),
			providercomment.ParentIDIsNil(),
			providercomment.StatusEQ(service.CommentStatusActive),
			providercomment.SentimentEQ(service.SentimentPositive),
		).
		Count(ctx)
	if err != nil {
		return nil, err
	}

	negCount, err := r.client.ProviderComment.Query().
		Where(
			providercomment.ProviderEQ(provider),
			providercomment.ParentIDIsNil(),
			providercomment.StatusEQ(service.CommentStatusActive),
			providercomment.SentimentEQ(service.SentimentNegative),
		).
		Count(ctx)
	if err != nil {
		return nil, err
	}

	ratio := 0.0
	if total > 0 {
		ratio = math.Round((float64(posCount)/float64(total))*1000) / 10 // e.g. 85.5%
	}

	return &service.ProviderCommentSummary{
		Provider:      provider,
		TotalComments: total,
		PositiveCount: posCount,
		NegativeCount: negCount,
		PositiveRatio: ratio,
	}, nil
}

func (r *providerCommentRepository) IncrementReplyCount(ctx context.Context, commentID int64) error {
	client := clientFromContext(ctx, r.client)
	return client.ProviderComment.UpdateOneID(commentID).
		AddReplyCount(1).
		Exec(ctx)
}

func (r *providerCommentRepository) SetVote(ctx context.Context, commentID, userID int64, voteType string) (*service.VoteResult, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	existingVote, err := tx.ProviderCommentVote.Query().
		Where(
			providercommentvote.CommentIDEQ(commentID),
			providercommentvote.UserIDEQ(userID),
		).
		First(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, err
	}

	comment, err := tx.ProviderComment.Query().
		Where(providercomment.IDEQ(commentID)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrCommentNotFound, nil)
	}

	likeDelta := 0
	dislikeDelta := 0
	finalVote := voteType

	if existingVote != nil {
		// User already voted
		if voteType == service.VoteTypeNone || voteType == existingVote.VoteType {
			// Remove vote
			if existingVote.VoteType == service.VoteTypeLike {
				likeDelta = -1
			} else if existingVote.VoteType == service.VoteTypeDislike {
				dislikeDelta = -1
			}
			if err := tx.ProviderCommentVote.DeleteOne(existingVote).Exec(ctx); err != nil {
				return nil, err
			}
			finalVote = service.VoteTypeNone
		} else {
			// Change vote from like -> dislike or dislike -> like
			if existingVote.VoteType == service.VoteTypeLike && voteType == service.VoteTypeDislike {
				likeDelta = -1
				dislikeDelta = 1
			} else if existingVote.VoteType == service.VoteTypeDislike && voteType == service.VoteTypeLike {
				likeDelta = 1
				dislikeDelta = -1
			}
			if err := tx.ProviderCommentVote.UpdateOne(existingVote).SetVoteType(voteType).Exec(ctx); err != nil {
				return nil, err
			}
		}
	} else {
		// New vote
		if voteType != service.VoteTypeNone {
			if voteType == service.VoteTypeLike {
				likeDelta = 1
			} else if voteType == service.VoteTypeDislike {
				dislikeDelta = 1
			}
			if err := tx.ProviderCommentVote.Create().
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

	if err := tx.ProviderComment.UpdateOneID(commentID).
		SetLikeCount(newLikes).
		SetDislikeCount(newDislikes).
		Exec(ctx); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &service.VoteResult{
		CommentID:    commentID,
		LikeCount:    newLikes,
		DislikeCount: newDislikes,
		UserVote:     finalVote,
	}, nil
}

func (r *providerCommentRepository) GetUserVotes(ctx context.Context, userID int64, commentIDs []int64) (map[int64]string, error) {
	if len(commentIDs) == 0 {
		return make(map[int64]string), nil
	}

	votes, err := r.client.ProviderCommentVote.Query().
		Where(
			providercommentvote.UserIDEQ(userID),
			providercommentvote.CommentIDIn(commentIDs...),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	m := make(map[int64]string, len(votes))
	for _, v := range votes {
		m[v.CommentID] = v.VoteType
	}
	return m, nil
}

func (r *providerCommentRepository) AdminListComments(ctx context.Context, provider, status, sentiment string, page, pageSize int) ([]service.ProviderComment, *pagination.PaginationResult, error) {
	q := r.client.ProviderComment.Query()

	if provider != "" {
		q = q.Where(providercomment.ProviderEQ(provider))
	}
	if status != "" {
		q = q.Where(providercomment.StatusEQ(status))
	}
	if sentiment != "" {
		q = q.Where(providercomment.SentimentEQ(sentiment))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	offset := (page - 1) * pageSize
	entities, err := q.Order(dbent.Desc(providercomment.FieldCreatedAt)).
		Limit(pageSize).
		Offset(offset).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	comments := make([]service.ProviderComment, len(entities))
	for i, e := range entities {
		comments[i] = *r.mapEntityToService(e, "")
	}

	pagResult := &pagination.PaginationResult{
		Total:    int64(total),
		Page:     page,
		PageSize: pageSize,
		Pages:    int(math.Ceil(float64(total) / float64(pageSize))),
	}

	return comments, pagResult, nil
}

func (r *providerCommentRepository) AdminSetStatus(ctx context.Context, id int64, status string) error {
	client := clientFromContext(ctx, r.client)
	return client.ProviderComment.UpdateOneID(id).
		SetStatus(status).
		Exec(ctx)
}

func (r *providerCommentRepository) mapEntityToService(e *dbent.ProviderComment, userVote string) *service.ProviderComment {
	if e == nil {
		return nil
	}

	emailMasked := ""
	if e.Edges.User != nil {
		emailMasked = service.MaskEmail(e.Edges.User.Email)
	}

	res := &service.ProviderComment{
		ID:              e.ID,
		Provider:        e.Provider,
		UserID:          e.UserID,
		UserEmailMasked: emailMasked,
		ParentID:        e.ParentID,
		Content:         e.Content,
		Sentiment:       e.Sentiment,
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
