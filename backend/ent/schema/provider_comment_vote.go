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

// ProviderCommentVote holds the schema definition for the ProviderCommentVote entity.
type ProviderCommentVote struct {
	ent.Schema
}

func (ProviderCommentVote) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "provider_comment_votes"},
	}
}

func (ProviderCommentVote) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ProviderCommentVote) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("comment_id").
			Comment("Target comment ID"),
		field.Int64("user_id").
			Comment("Voting user ID"),
		field.String("vote_type").
			MaxLen(20).
			NotEmpty().
			Comment("Vote type: like or dislike"),
	}
}

func (ProviderCommentVote) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("comment", ProviderComment.Type).
			Ref("votes").
			Field("comment_id").
			Unique().
			Required(),
		edge.From("user", User.Type).
			Ref("provider_comment_votes").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (ProviderCommentVote) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("comment_id", "user_id").Unique(),
		index.Fields("user_id"),
	}
}
