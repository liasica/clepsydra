package service

import (
	"context"
	"time"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/demand"
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

// List 按状态筛选需求，status 为空返回全部
func (s *Demand) List(ctx context.Context, status string) ([]*ent.Demand, error) {
	q := s.client.Demand.Query().Order(ent.Desc(demand.FieldID))
	if status != "" {
		q = q.Where(demand.StatusEQ(demand.Status(status)))
	}

	return q.All(ctx)
}

// Get 按 ID 查询需求
func (s *Demand) Get(ctx context.Context, id int) (*ent.Demand, error) {
	d, err := s.client.Demand.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}

	return d, err
}

// Create 创建需求，初始状态 draft
func (s *Demand) Create(ctx context.Context, actor Actor, title, description string, estimatedHalfDays int, plannedStart *time.Time) (*ent.Demand, error) {
	if title == "" {
		return nil, ErrBadRequest("标题不能为空")
	}
	if estimatedHalfDays <= 0 {
		return nil, ErrBadRequest("预估人天必须为正")
	}

	builder := s.client.Demand.Create().
		SetTitle(title).
		SetDescription(description).
		SetEstimatedHalfDays(estimatedHalfDays)
	if plannedStart != nil {
		builder.SetPlannedStartDate(*plannedStart)
	}

	d, err := builder.Save(ctx)
	if err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "demand.create", "demand", d.ID, map[string]any{
		"title": title, "estimated_half_days": estimatedHalfDays,
	})

	return d, nil
}

// Update 更新需求基本信息，仅 draft 与 pending_estimate 状态允许
func (s *Demand) Update(ctx context.Context, actor Actor, id int, title, description string, estimatedHalfDays int, plannedStart *time.Time) (*ent.Demand, error) {
	d, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if d.Status != demand.StatusDraft && d.Status != demand.StatusPendingEstimate {
		return nil, ErrInvalidTransition
	}
	if title == "" || estimatedHalfDays <= 0 {
		return nil, ErrBadRequest("标题不能为空且预估人天必须为正")
	}

	builder := d.Update().
		SetTitle(title).
		SetDescription(description).
		SetEstimatedHalfDays(estimatedHalfDays)
	if plannedStart != nil {
		builder.SetPlannedStartDate(*plannedStart)
	} else {
		builder.ClearPlannedStartDate()
	}

	d, err = builder.Save(ctx)
	if err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "demand.update", "demand", d.ID, map[string]any{
		"title": title, "estimated_half_days": estimatedHalfDays,
	})

	return d, nil
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

// SubmitEstimate 提交预估，进入待需求方确认人天
func (s *Demand) SubmitEstimate(ctx context.Context, actor Actor, id int) error {
	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	err = s.transit(ctx, id, d.Status, demand.StatusPendingEstimate, func(u *ent.DemandUpdate) {})
	if err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "demand.submit_estimate", "demand", id, nil)

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
	if actualEnd.After(time.Now()) {
		return ErrBadRequest("完成日期不能晚于当前时间")
	}

	// 完成日期所在账期已出账（非草稿）则拒绝，保证账期封闭、防止补录漏计费
	period := actualEnd.In(time.Local).Format("2006-01")
	var closed bool
	closed, err = s.client.Bill.Query().
		Where(bill.Period(period), bill.StatusNEQ(bill.StatusDraft)).
		Exist(ctx)
	if err != nil {
		return err
	}
	if closed {
		return ErrBadRequest("完成日期所在账期的账单已分享或确认，不可补录")
	}

	// 按设置计算确认截止时间
	window, err := s.setting.Int(ctx, SettingDemandConfirmWindow)
	if err != nil {
		return err
	}
	unit, err := s.setting.Str(ctx, SettingWindowUnit)
	if err != nil {
		return err
	}
	cal, err := s.setting.Calendar(ctx)
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
