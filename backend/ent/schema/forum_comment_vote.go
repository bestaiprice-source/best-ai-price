package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ForumCommentVote holds the schema definition for the ForumCommentVote entity.
type ForumCommentVote struct {
	ent.Schema
}

func (ForumCommentVote) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "forum_comment_votes"},
	}
}

func (ForumCommentVote) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ForumCommentVote) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("comment_id").
			Comment("Target forum comment ID"),
		field.Int64("user_id").
			Comment("Voting user ID"),
		field.String("vote_type").
			MaxLen(20).
			NotEmpty().
			Comment("Vote type: like or dislike"),
	}
}

func (ForumCommentVote) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("comment", ForumComment.Type).
			Ref("votes").
			Field("comment_id").
			Unique().
			Required(),
		edge.From("user", User.Type).
			Ref("forum_comment_votes").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (ForumCommentVote) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("comment_id", "user_id").
			Unique(),
	}
}
