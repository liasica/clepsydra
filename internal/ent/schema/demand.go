package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Demand 开发需求（项目），人天以半天数存储
type Demand struct {
	ent.Schema
}

func (Demand) Fields() []ent.Field {
	return []ent.Field{
		field.String("title"),
		field.Text("description").Optional(),
		field.Int("estimated_half_days").NonNegative(),
		field.Time("estimate_confirmed_at").Optional().Nillable(),
		field.Int("estimate_confirmed_by").Optional().Nillable(),
		field.Time("planned_start_date").Optional().Nillable(),
		field.Time("actual_start_date").Optional().Nillable(),
		field.Time("actual_end_date").Optional().Nillable(),
		field.Int("actual_half_days").Optional().Nillable(),
		field.Enum("status").
			Values("draft", "pending_estimate", "confirmed", "in_progress", "pending_acceptance", "accepted").
			Default("draft"),
		field.Time("accept_deadline").Optional().Nillable(),
		field.Time("accepted_at").Optional().Nillable(),
		field.Int("accepted_by").Optional().Nillable(),
		field.Bool("accept_auto").Default(false),
		field.Bool("accept_locked").Default(false), // 出账前锁定产生的自动确认
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Demand) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("actual_end_date"),
	}
}
