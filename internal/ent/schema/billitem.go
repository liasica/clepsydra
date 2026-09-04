package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BillItem 账单明细行，快照生成时的需求信息
type BillItem struct {
	ent.Schema
}

func (BillItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int("demand_id"),
		field.String("demand_title"),  // 快照
		field.String("demand_status"), // 快照
		field.Int("half_days"),        // 实际人天，缺省时取预估人天
		field.Int("amount"),           // 单位元，减免行为 0
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

func (BillItem) Indexes() []ent.Index {
	return []ent.Index{
		// 计费防重的数据库不变量：一个需求全局至多出现在一张账单里
		// service 层预检查存在并发窗口，唯一索引是最终防线
		index.Fields("demand_id").Unique(),
	}
}
