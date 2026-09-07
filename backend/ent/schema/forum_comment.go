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

// ForumComment holds the schema definition for the ForumComment entity.
type ForumComment struct {
	ent.Schema
}

func (ForumComment) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "forum_comments"},
	}
}

func (ForumComment) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ForumComment) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("topic_id").
			Comment("Target forum topic ID"),
		field.Int64("user_id").
			Comment("Author user ID"),
		field.Int64("parent_id").
			Optional().
			Nillable().
			Comment("Parent comment ID for replies, null for root topic comments"),
		field.String("content").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty().
			Comment("Comment text content"),
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
		field.Int("reply_count").
			Default(0).
			Comment("Total replies count"),
	}
}

func (ForumComment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("topic", ForumTopic.Type).
			Ref("comments").
			Field("topic_id").
			Unique().
			Required(),
		edge.From("user", User.Type).
			Ref("forum_comments").
			Field("user_id").
			Unique().
			Required(),
		edge.To("replies", ForumComment.Type).
			From("parent").
			Field("parent_id").
			Unique(),
		edge.To("votes", ForumCommentVote.Type),
	}
}

func (ForumComment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("topic_id", "status", "created_at"),
		index.Fields("parent_id"),
		index.Fields("user_id"),
	}
}
