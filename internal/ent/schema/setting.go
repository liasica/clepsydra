package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// Setting 键值设置
type Setting struct {
	ent.Schema
}

func (Setting) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").Unique(),
		field.String("value"),
	}
}
