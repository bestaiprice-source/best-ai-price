package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type ForumService struct {
	repo ForumRepository
}

func NewForumService(repo ForumRepository) *ForumService {
	return &ForumService{
		repo: repo,
	}
}

// -----------------------------------------------------------------------------
// Topics
// -----------------------------------------------------------------------------

func (s *ForumService) CreateTopic(ctx context.Context, input CreateForumTopicInput) (*ForumTopic, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, ErrForumTitleEmpty
	}
	if len([]rune(title)) > 255 {
		return nil, ErrForumTitleTooLong
	}

	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, ErrForumContentEmpty
	}
	if len([]rune(content)) > 10000 {
		return nil, ErrForumContentTooLong
	}

	topic := &ForumTopic{
		UserID:       input.UserID,
		Title:        title,
		Content:      content,
		Status:       ForumTopicStatusActive,
		LikeCount:    0,
		DislikeCount: 0,
		CommentCount: 0,
	}

	if err := s.repo.CreateTopic(ctx, topic); err != nil {
		return nil, err
	}

	return topic, nil
}

func (s *ForumService) GetTopic(ctx context.Context, id int64, currentUserID int64) (*ForumTopic, error) {
	topic, err := s.repo.GetTopicByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if topic.Status != ForumTopicStatusActive {
		return nil, ErrForumTopicNotFound
	}

	if currentUserID > 0 {
		userVotes, _ := s.repo.GetTopicUserVotes(ctx, currentUserID, []int64{id})
		if userVotes != nil {
			topic.UserVote = userVotes[id]
		}
	}

	return topic, nil
}

func (s *ForumService) ListTopics(ctx context.Context, query ForumTopicListQuery) ([]ForumTopic, *pagination.PaginationResult, error) {
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	return s.repo.ListTopics(ctx, query)
}

func (s *ForumService) VoteTopic(ctx context.Context, input VoteForumTopicInput) (*ForumVoteResult, error) {
	voteType := domain.NormalizeForumVoteType(input.VoteType)
	if voteType == "" {
		return nil, ErrForumInvalidVoteType
	}

	topic, err := s.repo.GetTopicByID(ctx, input.TopicID)
	if err != nil {
		return nil, ErrForumTopicNotFound
	}
	if topic.Status != ForumTopicStatusActive {
		return nil, ErrForumTopicNotFound
	}

	return s.repo.SetTopicVote(ctx, input.TopicID, input.UserID, voteType)
}

func (s *ForumService) AdminListTopics(ctx context.Context, status string, page, pageSize int) ([]ForumTopic, *pagination.PaginationResult, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return s.repo.AdminListTopics(ctx, strings.TrimSpace(status), page, pageSize)
}

func (s *ForumService) AdminSetTopicStatus(ctx context.Context, id int64, status string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != ForumTopicStatusActive && status != ForumTopicStatusHidden {
		return ErrForumTopicNotFound
	}
	return s.repo.AdminSetTopicStatus(ctx, id, status)
}

// -----------------------------------------------------------------------------
// Comments
// -----------------------------------------------------------------------------

func (s *ForumService) CreateComment(ctx context.Context, input CreateForumCommentInput) (*ForumComment, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, ErrForumContentEmpty
	}
	if len([]rune(content)) > 5000 {
		return nil, ErrForumContentTooLong
	}

	// Verify topic exists and is active
	topic, err := s.repo.GetTopicByID(ctx, input.TopicID)
	if err != nil {
		return nil, ErrForumTopicNotFound
	}
	if topic.Status != ForumTopicStatusActive {
		return nil, ErrForumTopicNotFound
	}

	var parentComment *ForumComment
	if input.ParentID != nil && *input.ParentID > 0 {
		parent, err := s.repo.GetCommentByID(ctx, *input.ParentID)
		if err != nil {
			return nil, ErrForumParentNotFound
		}
		if parent.TopicID != input.TopicID || parent.Status != ForumCommentStatusActive {
			return nil, ErrForumParentNotFound
		}
		parentComment = parent
	}

	comment := &ForumComment{
		TopicID:      input.TopicID,
		UserID:       input.UserID,
		ParentID:     input.ParentID,
		Content:      content,
		Status:       ForumCommentStatusActive,
		LikeCount:    0,
		DislikeCount: 0,
		ReplyCount:   0,
	}

	if err := s.repo.CreateComment(ctx, comment); err != nil {
		return nil, err
	}

	_ = s.repo.IncrementTopicCommentCount(ctx, input.TopicID)

	if parentComment != nil {
		_ = s.repo.IncrementCommentReplyCount(ctx, parentComment.ID)
	}

	return comment, nil
}

func (s *ForumService) GetCommentByID(ctx context.Context, id int64) (*ForumComment, error) {
	comment, err := s.repo.GetCommentByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if comment.Status != ForumCommentStatusActive {
		return nil, ErrForumCommentNotFound
	}
	return comment, nil
}

func (s *ForumService) ListCommentsByTopic(ctx context.Context, query ForumCommentListQuery) ([]ForumComment, *pagination.PaginationResult, error) {
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	// Verify topic exists
	if _, err := s.repo.GetTopicByID(ctx, query.TopicID); err != nil {
		return nil, nil, ErrForumTopicNotFound
	}

	return s.repo.ListCommentsByTopic(ctx, query)
}

func (s *ForumService) ListCommentReplies(ctx context.Context, parentID int64, page, pageSize int, currentUserID int64) ([]ForumComment, *pagination.PaginationResult, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// Verify parent exists
	if _, err := s.repo.GetCommentByID(ctx, parentID); err != nil {
		return nil, nil, ErrForumCommentNotFound
	}

	return s.repo.ListCommentReplies(ctx, parentID, page, pageSize, currentUserID)
}

func (s *ForumService) VoteComment(ctx context.Context, input VoteForumCommentInput) (*ForumVoteResult, error) {
	voteType := domain.NormalizeForumVoteType(input.VoteType)
	if voteType == "" {
		return nil, ErrForumInvalidVoteType
	}

	comment, err := s.repo.GetCommentByID(ctx, input.CommentID)
	if err != nil {
		return nil, ErrForumCommentNotFound
	}
	if comment.Status != ForumCommentStatusActive {
		return nil, ErrForumCommentNotFound
	}

	return s.repo.SetCommentVote(ctx, input.CommentID, input.UserID, voteType)
}

func (s *ForumService) AdminListComments(ctx context.Context, status string, page, pageSize int) ([]ForumComment, *pagination.PaginationResult, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return s.repo.AdminListComments(ctx, strings.TrimSpace(status), page, pageSize)
}

func (s *ForumService) AdminSetCommentStatus(ctx context.Context, id int64, status string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != ForumCommentStatusActive && status != ForumCommentStatusHidden {
		return ErrForumCommentNotFound
	}
	return s.repo.AdminSetCommentStatus(ctx, id, status)
}
