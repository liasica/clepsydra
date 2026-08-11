package service

import (
	"context"
	"time"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/demand"
	"clepsydra/internal/ent/project"
	"clepsydra/internal/workday"
)

// Demand 需求服务，管理需求全生命周期状态机
type Demand struct {
	client  *ent.Client
	setting *Setting
	audit   *Audit
}

// NewDemand 构建需求服务
func NewDemand(client *ent.Client, setting *Setting, audit *Audit) *Demand {
	return &Demand{client: client, setting: setting, audit: audit}
}

// transitions 状态机白名单：当前状态 → 允许进入的下一状态
var transitions = map[demand.Status][]demand.Status{
	demand.StatusDraft:             {demand.StatusPendingEstimate},
	demand.StatusPendingEstimate:   {demand.StatusConfirmed},
	demand.StatusConfirmed:         {demand.StatusInProgress},
	demand.StatusInProgress:        {demand.StatusPendingAcceptance},
	demand.StatusPendingAcceptance: {demand.StatusAccepted},
}

// canTransit 判定状态流转是否合法
func canTransit(from, to demand.Status) bool {
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}

	return false
}

// normalizePriority 校验优先级合法性，空串按默认 normal 处理
func normalizePriority(priority string) (demand.Priority, error) {
	if priority == "" {
		return demand.PriorityNormal, nil
	}

	p := demand.Priority(priority)
	if err := demand.PriorityValidator(p); err != nil {
		return "", ErrBadRequest("优先级不合法")
	}

	return p, nil
}

// List 按状态、项目与优先级筛选需求，status/priority 为空、projectID 为 0 表示不筛选；预加载项目标签
func (s *Demand) List(ctx context.Context, status string, projectID int, priority string) ([]*ent.Demand, error) {
	q := s.client.Demand.Query().WithProjects().Order(ent.Desc(demand.FieldID))
	if status != "" {
		q = q.Where(demand.StatusEQ(demand.Status(status)))
	}
	if projectID > 0 {
		q = q.Where(demand.HasProjectsWith(project.ID(projectID)))
	}
	if priority != "" {
		q = q.Where(demand.PriorityEQ(demand.Priority(priority)))
	}

	return q.All(ctx)
}

// Get 按 ID 查询需求，预加载项目标签
func (s *Demand) Get(ctx context.Context, id int) (*ent.Demand, error) {
	d, err := s.client.Demand.Query().
		Where(demand.ID(id)).
		WithProjects().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}

	return d, err
}

// Create 创建需求；预估人天为超管专属的可选快捷路径：
// 填了人天创建即进入 pending_estimate（等价创建 + 提交预估一步完成），
// confirmed 再直达 confirmed（等价超管代确认，确认人记为创建者本人）。
// INSERT 一次性写入终态，无并发流转问题，故不经过 transit 状态机。
// 角色权限与「日期 / 已确认必须依附人天」由 handler 层校验，这里只保留业务不变量防御
func (s *Demand) Create(ctx context.Context, actor Actor, title, description string, estimatedHalfDays int, plannedStart *time.Time, confirmed bool, projectIDs []int, priority string) (*ent.Demand, error) {
	if title == "" {
		return nil, ErrBadRequest("标题不能为空")
	}
	if estimatedHalfDays < 0 {
		return nil, ErrBadRequest("预估人天不可为负")
	}
	if confirmed && estimatedHalfDays == 0 {
		return nil, ErrBadRequest("勾选已确认时预估人天必须为正")
	}

	prio, err := normalizePriority(priority)
	if err != nil {
		return nil, err
	}

	ids, err := s.normalizeProjectIDs(ctx, projectIDs)
	if err != nil {
		return nil, err
	}

	create := s.client.Demand.Create().
		SetTitle(title).
		SetDescription(description).
		SetEstimatedHalfDays(estimatedHalfDays).
		SetPriority(prio).
		AddProjectIDs(ids...)

	now := time.Now()
	switch {
	case confirmed:
		create.SetStatus(demand.StatusConfirmed).
			SetEstimateConfirmedAt(now).
			SetEstimateConfirmedBy(actor.ID)
	case estimatedHalfDays > 0:
		create.SetStatus(demand.StatusPendingEstimate)
	}
	if plannedStart != nil {
		create.SetPlannedStartDate(*plannedStart)
	}

	d, err := create.Save(ctx)
	if err != nil {
		// 校验通过后写入前项目被并发删除会触发外键约束冲突，转为业务错误而非 500
		if len(ids) > 0 && ent.IsConstraintError(err) {
			return nil, ErrBadRequest("项目不存在")
		}

		return nil, err
	}

	payload := map[string]any{
		"title":               title,
		"estimated_half_days": estimatedHalfDays,
		"confirmed":           confirmed,
	}
	if prio != demand.DefaultPriority {
		payload["priority"] = string(prio)
	}
	if plannedStart != nil {
		payload["planned_start_date"] = plannedStart.Format("2006-01-02")
	}
	if len(ids) > 0 {
		payload["project_ids"] = ids
	}
	s.audit.Record(ctx, actor, "demand.create", "demand", d.ID, payload)
	// 创建即确认补写确认审计，避免审计时间线里 confirmed 状态凭空出现
	if confirmed {
		s.audit.Record(ctx, actor, "demand.confirm_estimate", "demand", d.ID, nil)
	}

	return d, nil
}

// UpdateProjects 全量覆盖需求的项目标签，任何状态均可：
// 标签是归类元数据，不影响人天与账单金额，存量已完成需求也要能补打标签
func (s *Demand) UpdateProjects(ctx context.Context, actor Actor, id int, projectIDs []int) (*ent.Demand, error) {
	ids, err := s.normalizeProjectIDs(ctx, projectIDs)
	if err != nil {
		return nil, err
	}

	err = s.client.Demand.UpdateOneID(id).
		ClearProjects().
		AddProjectIDs(ids...).
		Exec(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		// 校验通过后写入前项目被并发删除会触发外键约束冲突，转为业务错误而非 500
		if len(ids) > 0 && ent.IsConstraintError(err) {
			return nil, ErrBadRequest("项目不存在")
		}

		return nil, err
	}

	s.audit.Record(ctx, actor, "demand.update_projects", "demand", id, map[string]any{
		"project_ids": ids,
	})

	return s.Get(ctx, id)
}

// UpdatePriority 调整需求优先级，任何状态均可：
// 优先级是排期参考元数据，不影响人天与账单金额
func (s *Demand) UpdatePriority(ctx context.Context, actor Actor, id int, priority string) (*ent.Demand, error) {
	if priority == "" {
		return nil, ErrBadRequest("优先级不能为空")
	}

	prio, err := normalizePriority(priority)
	if err != nil {
		return nil, err
	}

	err = s.client.Demand.UpdateOneID(id).
		SetPriority(prio).
		Exec(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "demand.update_priority", "demand", id, map[string]any{
		"priority": string(prio),
	})

	return s.Get(ctx, id)
}

// normalizeProjectIDs 去重并校验项目 ID 均存在，空切片直接通过
func (s *Demand) normalizeProjectIDs(ctx context.Context, ids []int) ([]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	seen := make(map[int]bool, len(ids))
	uniq := make([]int, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}

	n, err := s.client.Project.Query().Where(project.IDIn(uniq...)).Count(ctx)
	if err != nil {
		return nil, err
	}
	if n != len(uniq) {
		return nil, ErrBadRequest("项目不存在")
	}

	return uniq, nil
}

// Update 更新需求标题与描述（markdown 原文），仅 draft 与 pending_estimate 状态允许
func (s *Demand) Update(ctx context.Context, actor Actor, id int, title, description string) (*ent.Demand, error) {
	if title == "" {
		return nil, ErrBadRequest("标题不能为空")
	}

	// 状态检查随 UPDATE 语句条件化（Where 带状态谓词 + n==0 判定），避免 Get 后无条件写入的 TOCTOU
	n, err := s.client.Demand.Update().
		Where(demand.ID(id), demand.StatusIn(demand.StatusDraft, demand.StatusPendingEstimate)).
		SetTitle(title).
		SetDescription(description).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		// 区分「需求不存在」与「状态不允许更新」，保持原有 404 / 422 语义
		if _, getErr := s.Get(ctx, id); getErr != nil {
			return nil, getErr
		}

		return nil, ErrInvalidTransition
	}

	d, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "demand.update", "demand", d.ID, map[string]any{
		"title": title,
	})

	return d, nil
}

// Delete 软删除需求，任何状态都允许
//
// 删除后需求不再出现在列表、工作台统计与账单生成范围里。账单明细存的是快照，
// 已出的账单金额不受影响；明细里的 demand_id 仍指向保留下来的记录，
// 需要追溯时用 schema.SkipSoftDelete 查询即可
func (s *Demand) Delete(ctx context.Context, actor Actor, id int) error {
	// 先取一次拿标题与状态写审计，同时借 Get 的软删除过滤挡掉重复删除
	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	if err = s.client.Demand.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return ErrNotFound
		}

		return err
	}

	s.audit.Record(ctx, actor, "demand.delete", "demand", id, map[string]any{
		"title":  d.Title,
		"status": d.Status.String(),
	})

	return nil
}

// transit 通用状态流转：条件更新防止并发下重复流转
func (s *Demand) transit(ctx context.Context, id int, from, to demand.Status, apply func(*ent.DemandUpdate)) error {
	if !canTransit(from, to) {
		return ErrInvalidTransition
	}

	update := s.client.Demand.Update().
		Where(demand.ID(id), demand.StatusEQ(from)).
		SetStatus(to)
	apply(update)

	n, err := update.Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidTransition
	}

	return nil
}

// SubmitEstimate 提交预估人天与预计开工并进入待需求方确认；pending_estimate 下可重复提交修正
func (s *Demand) SubmitEstimate(ctx context.Context, actor Actor, id int, estimatedHalfDays int, plannedStart *time.Time) error {
	if estimatedHalfDays <= 0 {
		return ErrBadRequest("预估人天必须为正")
	}

	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	apply := func(u *ent.DemandUpdate) {
		u.SetEstimatedHalfDays(estimatedHalfDays)
		if plannedStart != nil {
			u.SetPlannedStartDate(*plannedStart)
		} else {
			u.ClearPlannedStartDate()
		}
	}

	switch d.Status {
	case demand.StatusDraft:
		err = s.transit(ctx, id, d.Status, demand.StatusPendingEstimate, apply)
	case demand.StatusPendingEstimate:
		// 状态不变，仅修正预估数据；条件更新防止并发下状态已流转
		update := s.client.Demand.Update().
			Where(demand.ID(id), demand.StatusEQ(demand.StatusPendingEstimate))
		apply(update)
		var n int
		n, err = update.Save(ctx)
		if err == nil && n == 0 {
			err = ErrInvalidTransition
		}
	default:
		err = ErrInvalidTransition
	}
	if err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "demand.submit_estimate", "demand", id, map[string]any{
		"estimated_half_days": estimatedHalfDays,
	})

	return nil
}

// ConfirmEstimate 需求方确认预估人天
func (s *Demand) ConfirmEstimate(ctx context.Context, actor Actor, id int) error {
	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	err = s.transit(ctx, id, d.Status, demand.StatusConfirmed, func(u *ent.DemandUpdate) {
		u.SetEstimateConfirmedAt(now).SetEstimateConfirmedBy(actor.ID)
	})
	if err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "demand.confirm_estimate", "demand", id, nil)

	return nil
}

// Start 标记开工
func (s *Demand) Start(ctx context.Context, actor Actor, id int, actualStart time.Time) error {
	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	err = s.transit(ctx, id, d.Status, demand.StatusInProgress, func(u *ent.DemandUpdate) {
		u.SetActualStartDate(actualStart)
	})
	if err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "demand.start", "demand", id, map[string]any{
		"actual_start_date": actualStart.Format("2006-01-02"),
	})

	return nil
}

// Finish 标记完成：写入实际日期与人天，计算需求方确认截止时间
func (s *Demand) Finish(ctx context.Context, actor Actor, id int, actualStart, actualEnd time.Time, actualHalfDays int) error {
	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if actualHalfDays <= 0 {
		return ErrBadRequest("实际人天必须为正")
	}
	if actualEnd.Before(actualStart) {
		return ErrBadRequest("完成日期不能早于开工日期")
	}

	// 按设置计算确认截止时间
	var window int
	window, err = s.setting.Int(ctx, SettingDemandConfirmWindow)
	if err != nil {
		return err
	}
	var unit string
	unit, err = s.setting.Str(ctx, SettingWindowUnit)
	if err != nil {
		return err
	}
	var cal *workday.Calendar
	cal, err = s.setting.Calendar(ctx)
	if err != nil {
		return err
	}
	deadline := cal.Deadline(time.Now(), window, workday.Unit(unit))

	err = s.transit(ctx, id, d.Status, demand.StatusPendingAcceptance, func(u *ent.DemandUpdate) {
		u.SetActualStartDate(actualStart).
			SetActualEndDate(actualEnd).
			SetActualHalfDays(actualHalfDays).
			SetAcceptDeadline(deadline)
	})
	if err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "demand.finish", "demand", id, map[string]any{
		"actual_end_date": actualEnd.Format("2006-01-02"), "actual_half_days": actualHalfDays,
	})

	return nil
}

// Accept 确认完成：auto 表示逾期自动确认，locked 表示出账前锁定
func (s *Demand) Accept(ctx context.Context, actor Actor, id int, auto, locked bool) error {
	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	err = s.transit(ctx, id, d.Status, demand.StatusAccepted, func(u *ent.DemandUpdate) {
		u.SetAcceptedAt(now).SetAcceptedBy(actor.ID).SetAcceptAuto(auto).SetAcceptLocked(locked)
	})
	if err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "demand.accept", "demand", id, map[string]any{
		"auto": auto, "locked": locked,
	})

	return nil
}
