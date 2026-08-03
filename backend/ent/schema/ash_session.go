package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// AshSession holds the schema definition for the AshSession entity.
type AshSession struct {
	ent.Schema
}

// Fields of the AshSession.
func (AshSession) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			MaxLen(64).
			Unique(),
		field.String("owner_id").
			MaxLen(64).
			NotEmpty(),
		field.String("conversation_id").
			MaxLen(64).
			Optional(),
		field.String("state").
			MaxLen(32).
			NotEmpty(),
		field.Text("summary").
			Optional(),
		field.JSON("metadata", map[string]interface{}{}).
			Default(map[string]interface{}{}),
		field.Time("created_at").
			Default(time.Now),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the AshSession.
func (AshSession) Edges() []ent.Edge {
	return nil
}
