package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// Holiday 节假日与调休补班日
type Holiday struct {
	ent.Schema
}

func (Holiday) Fields() []ent.Field {
	return []ent.Field{
		field.String("date").Unique(), // 格式 2026-01-01
		field.Enum("type").Values("holiday", "workday"),
		field.String("name").Optional(),
	}
}
