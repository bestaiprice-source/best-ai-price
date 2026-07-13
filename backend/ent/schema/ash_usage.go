package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type AshUsage struct {
	ent.Schema
}

func (AshUsage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ash_usages"},
	}
}

func (AshUsage) Fields() []ent.Field {
	return []ent.Field{
		field.String("owner_id").
			MaxLen(64).
			NotEmpty(),
		field.String("session_id").
			MaxLen(64).
			Optional().
			Nillable(),
		field.String("node_id").
			MaxLen(64).
			Optional().
			Nillable(),
		field.Int64("input_tokens").
			Default(0),
		field.Int64("output_tokens").
			Default(0),
		field.Int64("cost_units").
			Default(0),
		field.Time("created_at").
			Default(time.Now),
	}
}
