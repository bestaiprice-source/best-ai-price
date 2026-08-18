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

// ProviderComment holds the schema definition for the ProviderComment entity.
type ProviderComment struct {
	ent.Schema
}

func (ProviderComment) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "provider_comments"},
	}
}

func (ProviderComment) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ProviderComment) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").
			MaxLen(100).
			NotEmpty().
			Comment("Provider identifier, e.g. yunwu, claude, openai"),
		field.Int64("user_id").
			Comment("Author user ID"),
		field.Int64("parent_id").
			Optional().
			Nillable().
			Comment("Parent comment ID for replies, null for root provider reviews"),
		field.String("content").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty().
			Comment("Comment text content"),
		field.String("sentiment").
			MaxLen(20).
			Default("positive").
			Comment("Sentiment: positive or negative (or neutral for replies)"),
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

func (ProviderComment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("provider_comments").
			Field("user_id").
			Unique().
			Required(),
		edge.To("replies", ProviderComment.Type).
			From("parent").
			Field("parent_id").
			Unique(),
		edge.To("votes", ProviderCommentVote.Type),
	}
}

func (ProviderComment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "status", "created_at"),
		index.Fields("provider", "sentiment", "status"),
		index.Fields("parent_id"),
		index.Fields("user_id"),
	}
}
