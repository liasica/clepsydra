package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AuditLog 审计日志，只增不改不删，是合同效力的操作依据
type AuditLog struct {
	ent.Schema
}

func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int("actor_id"),      // 0 表示系统自动操作
		field.String("actor_name"), // 快照，系统操作为 system
		field.String("action"),     // 如 demand.accept、bill.share
		field.String("target_type"),
		field.Int("target_id"),
		field.JSON("detail", map[string]any{}).Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (AuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("target_type", "target_id"),
	}
}
