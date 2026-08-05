package service

import (
	"context"

	"github.com/rs/zerolog/log"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/auditlog"
)

// Actor 操作者信息
type Actor struct {
	ID   int
	Name string
}

// SystemActor 系统自动操作者
var SystemActor = Actor{ID: 0, Name: "system"}

// Audit 审计服务，记录所有关键操作
type Audit struct {
	client *ent.Client
}

// NewAudit 构建审计服务
func NewAudit(client *ent.Client) *Audit {
	return &Audit{client: client}
}

// Record 写入审计日志，失败仅记录错误日志不阻断业务
func (a *Audit) Record(ctx context.Context, actor Actor, action, targetType string, targetID int, detail map[string]any) {
	builder := a.client.AuditLog.Create().
		SetActorID(actor.ID).
		SetActorName(actor.Name).
		SetAction(action).
		SetTargetType(targetType).
		SetTargetID(targetID)

	if detail != nil {
		builder.SetDetail(detail)
	}

	if _, err := builder.Save(ctx); err != nil {
		log.Error().Err(err).Str("action", action).Msg("写入审计日志失败")
	}
}

// List 分页查询审计日志，targetType/action/targetID 为空或 0 时不过滤
// page 从 1 起，size 上限 100，按 id 倒序
func (a *Audit) List(ctx context.Context, targetType, action string, targetID, page, size int) (int, []*ent.AuditLog, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}

	q := a.client.AuditLog.Query()
	if targetType != "" {
		q = q.Where(auditlog.TargetType(targetType))
	}
	if action != "" {
		q = q.Where(auditlog.Action(action))
	}
	if targetID > 0 {
		q = q.Where(auditlog.TargetID(targetID))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return 0, nil, err
	}

	var rows []*ent.AuditLog
	rows, err = q.Order(ent.Desc(auditlog.FieldID)).Offset((page - 1) * size).Limit(size).All(ctx)
	if err != nil {
		return 0, nil, err
	}

	return total, rows, nil
}
