package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Demand 开发需求（项目），人天以半天数存储
type Demand struct {
	ent.Schema
}

// Mixin 需求支持软删除：删除后不再出现在任何列表与统计里，但记录保留，
// 账单明细里的 demand_id 仍能追溯到原始需求
func (Demand) Mixin() []ent.Mixin {
	return []ent.Mixin{
		SoftDeleteMixin{},
	}
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

// Edges 需求与项目 / 标签的多对多关联：均为轻量归类元数据，不影响人天与账单金额
func (Demand) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("projects", Project.Type).Ref("demands"),
		edge.From("tags", Tag.Type).Ref("demands"),
	}
}

func (Demand) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("actual_end_date"),
	}
}
