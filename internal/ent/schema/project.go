package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Project 项目，轻量标签形态：需求可归属多个项目，用于筛选归类
// 不做软删除：删除即物理删除，与需求的关联由中间表外键级联清除
type Project struct {
	ent.Schema
}

func (Project) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Unique(),
		field.String("color").Optional(), // antdv 预设 tag 色名（如 blue），空串表示默认色
		field.Text("remark").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Project) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("demands", Demand.Type),
	}
}
