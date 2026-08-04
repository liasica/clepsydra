package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Bill 月度账单，period 形如 2026-07，全局唯一
type Bill struct {
	ent.Schema
}

func (Bill) Fields() []ent.Field {
	return []ent.Field{
		field.String("period").Unique(),
		field.Enum("status").Values("draft", "pending", "confirmed").Default("draft"),
		field.Int("daily_rate"), // 生成时快照，单位元
		field.Int("base_fee"),   // 生成时快照，单位元
		field.Int("total_half_days"),
		field.Int("total_amount"), // 单位元
		field.Time("shared_at").Optional().Nillable(),
		field.Time("confirm_deadline").Optional().Nillable(),
		field.Time("confirmed_at").Optional().Nillable(),
		field.Int("confirmed_by").Optional().Nillable(),
		field.Bool("confirm_auto").Default(false),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Bill) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("items", BillItem.Type),
	}
}
