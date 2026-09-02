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

// ForumPostVote holds the schema definition for the ForumPostVote entity.
type ForumPostVote struct {
	ent.Schema
}

func (ForumPostVote) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "forum_post_votes"},
	}
}

func (ForumPostVote) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ForumPostVote) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("post_id").
			Comment("Target forum post ID"),
		field.Int64("user_id").
			Comment("Voting user ID"),
		field.String("vote_type").
			MaxLen(20).
			NotEmpty().
			Comment("Vote type: like or dislike"),
	}
}

func (ForumPostVote) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("post", ForumPost.Type).
			Ref("votes").
			Field("post_id").
			Unique().
			Required(),
		edge.From("user", User.Type).
			Ref("forum_post_votes").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (ForumPostVote) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("post_id", "user_id").
			Unique(),
	}
}
