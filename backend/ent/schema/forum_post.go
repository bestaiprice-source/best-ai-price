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

// ForumPost holds the schema definition for the ForumPost entity.
type ForumPost struct {
	ent.Schema
}

func (ForumPost) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "forum_posts"},
	}
}

func (ForumPost) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ForumPost) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").
			Comment("Author user ID"),
		field.Int64("parent_id").
			Optional().
			Nillable().
			Comment("Parent post ID for replies, null for root forum posts"),
		field.String("content").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty().
			Comment("Post text content"),
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

func (ForumPost) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("forum_posts").
			Field("user_id").
			Unique().
			Required(),
		edge.To("replies", ForumPost.Type).
			From("parent").
			Field("parent_id").
			Unique(),
		edge.To("votes", ForumPostVote.Type),
	}
}

func (ForumPost) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at"),
		index.Fields("parent_id"),
		index.Fields("user_id"),
	}
}
