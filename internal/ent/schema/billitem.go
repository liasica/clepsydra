package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// BillItem 账单明细行，快照生成时的需求信息
type BillItem struct {
	ent.Schema
}

func (BillItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int("demand_id"),
		field.String("demand_title"),   // 快照
		field.String("demand_status"),  // 快照
		field.Int("half_days"),         // 计费行为实际人天，展示行为预估人天
		field.Int("amount"),            // 单位元，展示行与减免行为 0
		field.Bool("billable"),         // true 计费行，false 展示行
		field.Bool("waived").Default(false),
		field.Time("planned_start_date").Optional().Nillable(), // 快照
		field.String("note").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (BillItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("bill", Bill.Type).Ref("items").Unique().Required(),
	}
}
