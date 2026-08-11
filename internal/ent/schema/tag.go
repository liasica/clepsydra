package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Tag 需求性质标签（如新功能、缺陷修复、优化），颜色创建时按名称生成并固化，改名不重算
// 不做软删除：删除即物理删除，与需求的关联由中间表外键级联清除
type Tag struct {
	ent.Schema
}

func (Tag) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Unique(),
		field.String("color"), // 十六进制色值（如 #3b82f6），由服务端生成，不接受外部传入
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Tag) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("demands", Demand.Type),
	}
}
