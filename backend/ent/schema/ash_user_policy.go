package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AshUserPolicy is the durable source of truth for fair-share access policy.
type AshUserPolicy struct{ ent.Schema }

func (AshUserPolicy) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "ash_user_policies"}}
}

func (AshUserPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.String("owner_id").MaxLen(64).NotEmpty(),
		field.String("pool_type").MaxLen(32).NotEmpty(),
		field.String("plan_id").MaxLen(64).Default("pool-basic"),
		field.Int64("monthly_limit").NonNegative().Default(0),
		field.Int64("daily_burst").NonNegative().Default(0),
		field.Int64("monthly_limit_micro_usd").NonNegative().Default(0),
		field.Int64("daily_burst_micro_usd").NonNegative().Default(0),
		field.Int("priority").Positive().Default(1),
		field.String("deposit_state").MaxLen(32).Default(""),
		field.Bool("is_active").Default(true),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (AshUserPolicy) Indexes() []ent.Index {
	return []ent.Index{index.Fields("owner_id", "pool_type").Unique()}
}
