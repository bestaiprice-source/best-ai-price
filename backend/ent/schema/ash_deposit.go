package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AshDeposit struct {
	ent.Schema
}

func (AshDeposit) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ash_deposits"},
	}
}

func (AshDeposit) Fields() []ent.Field {
	return []ent.Field{
		field.String("owner_id").
			MaxLen(64).
			NotEmpty(),
		field.Int64("amount_cents").
			Positive(),
		field.String("currency").
			MaxLen(8).
			Default("USD"),
		field.String("state").
			MaxLen(32).
			Default("held"),
		field.Int("notice_days").
			Default(10),
		field.Time("created_at").
			Default(time.Now),
		field.Time("released_at").
			Optional().
			Nillable(),
	}
}

func (AshDeposit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("owner_id"),
		index.Fields("state"),
	}
}
