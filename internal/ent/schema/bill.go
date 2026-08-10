package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Bill 账单，自动账单按账期（period）幂等生成，手动账单无账期
type Bill struct {
	ent.Schema
}

func (Bill) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"), // 账单名称，自动账单为「自动生成：YYYY-MM」
		field.String("period").Optional().Nillable().Unique(), // 自动账单账期 YYYY-MM，手动账单为空；唯一约束保证自动生成幂等
		field.Enum("status").Values("pending", "unpaid", "paid").Default("pending"),
		field.Int("daily_rate"), // 生成时快照，单位元
		field.Int("base_fee"),   // 生成时快照，单位元，手动账单为 0
		field.Int("total_half_days"),
		field.Int("total_amount"),                   // 单位元
		field.Bool("total_override").Default(false), // 总额被手工指定后置位，此后重算只更新人天合计不再触碰总额
		field.Time("confirm_deadline").Optional().Nillable(),
		field.Time("confirmed_at").Optional().Nillable(),
		field.Int("confirmed_by").Optional().Nillable(),
		field.Bool("confirm_auto").Default(false),
		field.Time("paid_at").Optional().Nillable(),
		field.Int("paid_by").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Bill) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("items", BillItem.Type),
	}
}
