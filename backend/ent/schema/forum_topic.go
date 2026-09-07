package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ForumTopic holds the schema definition for the ForumTopic entity.
type ForumTopic struct {
	ent.Schema
}

func (ForumTopic) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "forum_topics"},
	}
}

func (ForumTopic) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ForumTopic) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").
			Comment("Author user ID"),
		field.String("title").
			MaxLen(255).
			NotEmpty().
			Comment("Topic title"),
		field.String("content").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty().
			Comment("Topic description / content"),
		field.String("status").
			MaxLen(20).
			Default("active").
			Comment("Status: active, hidden"),
		field.Int("like_count").
			Default(0).
			Comment("Total likes count"),
		field.Int("dislike_count").
			Default(0).
			Comment("Total dislikes count"),
		field.Int("comment_count").
			Default(0).
			Comment("Total comments count"),
	}
}

func (ForumTopic) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("forum_topics").
			Field("user_id").
			Unique().
			Required(),
		edge.To("comments", ForumComment.Type),
		edge.To("votes", ForumTopicVote.Type),
	}
}

func (ForumTopic) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at"),
		index.Fields("user_id"),
	}
}
