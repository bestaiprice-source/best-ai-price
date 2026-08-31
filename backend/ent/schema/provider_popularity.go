package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ProviderPopularity holds the schema definition for the ProviderPopularity entity.
type ProviderPopularity struct {
	ent.Schema
}

func (ProviderPopularity) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "provider_popularity"},
	}
}

func (ProviderPopularity) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ProviderPopularity) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").
			MaxLen(100).
			NotEmpty().
			Unique().
			Comment("Provider identifier, e.g. openai, claude, gemini, yunwu"),
		field.Int64("click_count").
			Default(0).
			Comment("Total verified clicks count"),
		field.Time("last_clicked_at").
			Optional().
			Nillable().
			Default(time.Now).
			Comment("Timestamp of the last verified click"),
	}
}

func (ProviderPopularity) Edges() []ent.Edge {
	return nil
}

func (ProviderPopularity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider").Unique(),
		index.Fields("click_count"),
	}
}
