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

// ForumTopicVote holds the schema definition for the ForumTopicVote entity.
type ForumTopicVote struct {
	ent.Schema
}

func (ForumTopicVote) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "forum_topic_votes"},
	}
}

func (ForumTopicVote) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ForumTopicVote) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("topic_id").
			Comment("Target forum topic ID"),
		field.Int64("user_id").
			Comment("Voting user ID"),
		field.String("vote_type").
			MaxLen(20).
			NotEmpty().
			Comment("Vote type: like or dislike"),
	}
}

func (ForumTopicVote) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("topic", ForumTopic.Type).
			Ref("votes").
			Field("topic_id").
			Unique().
			Required(),
		edge.From("user", User.Type).
			Ref("forum_topic_votes").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (ForumTopicVote) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("topic_id", "user_id").
			Unique(),
	}
}
