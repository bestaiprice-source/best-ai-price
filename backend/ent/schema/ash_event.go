package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type AshEvent struct {
	ent.Schema
}

func (AshEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ash_events"},
	}
}

func (AshEvent) Fields() []ent.Field {
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
		field.String("event_type").
			MaxLen(64).
			NotEmpty(),
		field.JSON("payload", map[string]any{}),
		field.Time("created_at").
			Default(time.Now),
	}
}
