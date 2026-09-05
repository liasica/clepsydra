package handler

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/ent"
	"clepsydra/internal/service"
)

// Bill 账单接口
type Bill struct {
	svc *service.Bill
}

// NewBill 构建账单 handler
func NewBill(svc *service.Bill) *Bill {
	return &Bill{svc: svc}
}

// detail 组装含需求当前状态及项目标签的账单详情响应
func (h *Bill) detail(c echo.Context, b *ent.Bill) error {
	demands, err := h.svc.ItemDemands(c.Request().Context(), b.Edges.Items)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, newBillDetailDTO(b, demands))
}

// parseItemID 解析路径中的账单明细 ID
func parseItemID(c echo.Context) (int, error) {
	id, err := strconv.Atoi(c.Param("itemId"))
	if err != nil || id <= 0 {
		return 0, service.ErrBadRequest("明细 ID 不合法")
	}

	return id, nil
}

// List GET /api/bills
func (h *Bill) List(c echo.Context) error {
	bills, err := h.svc.List(c.Request().Context())
	if err != nil {
		return api.Fail(c, err)
	}

	dtos := make([]billDTO, 0, len(bills))
	for _, b := range bills {
		dtos = append(dtos, newBillDTO(b))
	}

	return api.OK(c, dtos)
}

// Get GET /api/bills/:id
func (h *Bill) Get(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	b, err := h.svc.Get(c.Request().Context(), id)
	if err != nil {
		return api.Fail(c, err)
	}

	return h.detail(c, b)
}

// ToggleWaive POST /api/bills/:id/items/:itemId/waive
func (h *Bill) ToggleWaive(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	itemID, err := parseItemID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	if err = h.svc.ToggleWaive(c.Request().Context(), actor(c), id, itemID); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// Confirm POST /api/bills/:id/confirm
// 路由层仅要求登录态，人工确认固定传 auto=false
func (h *Bill) Confirm(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	if err = h.svc.Confirm(c.Request().Context(), actor(c), id, false); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// manualRequest 创建账单请求体
type manualRequest struct {
	Name      string `json:"name"`
	DemandIDs []int  `json:"demand_ids"`
}

// addItemRequest 添加账单明细请求体
type addItemRequest struct {
	DemandID int `json:"demand_id"`
}

// CreateManual POST /api/bills/manual
func (h *Bill) CreateManual(c echo.Context) error {
	var req manualRequest
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	b, err := h.svc.CreateManual(c.Request().Context(), actor(c), req.Name, req.DemandIDs)
	if err != nil {
		return api.Fail(c, err)
	}

	full, err := h.svc.Get(c.Request().Context(), b.ID)
	if err != nil {
		return api.Fail(c, err)
	}

	return h.detail(c, full)
}

// SelectableDemands GET /api/bills/selectable-demands
func (h *Bill) SelectableDemands(c echo.Context) error {
	demands, err := h.svc.SelectableDemands(c.Request().Context())
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, demands)
}

// AddItem POST /api/bills/:id/items
func (h *Bill) AddItem(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req addItemRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	if err = h.svc.AddItem(c.Request().Context(), actor(c), id, req.DemandID); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// RemoveItem DELETE /api/bills/:id/items/:itemId
func (h *Bill) RemoveItem(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	itemID, err := parseItemID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	if err = h.svc.RemoveItem(c.Request().Context(), actor(c), id, itemID); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// Pay POST /api/bills/:id/pay
func (h *Bill) Pay(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	if err = h.svc.Pay(c.Request().Context(), actor(c), id); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// updateBillRequest 编辑账单请求体，缺省字段不修改
type updateBillRequest struct {
	Name            *string `json:"name"`
	DailyRate       *int    `json:"daily_rate"`
	BaseFee         *int    `json:"base_fee"`
	ConfirmDeadline *string `json:"confirm_deadline"` // RFC3339 时间
	TotalAmount     *int    `json:"total_amount"`
	ResetTotal      bool    `json:"reset_total"`
}

// updateItemRequest 编辑账单明细请求体，缺省字段不修改
type updateItemRequest struct {
	HalfDays *int    `json:"half_days"`
	Amount   *int    `json:"amount"`
	Note     *string `json:"note"`
}

// Update PATCH /api/bills/:id
func (h *Bill) Update(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req updateBillRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	patch := service.BillUpdatePatch{
		Name:        req.Name,
		DailyRate:   req.DailyRate,
		BaseFee:     req.BaseFee,
		TotalAmount: req.TotalAmount,
		ResetTotal:  req.ResetTotal,
	}
	if req.ConfirmDeadline != nil {
		var deadline time.Time
		deadline, err = time.Parse(time.RFC3339, *req.ConfirmDeadline)
		if err != nil {
			return api.Fail(c, service.ErrBadRequest("确认截止时间格式不合法"))
		}
		patch.ConfirmDeadline = &deadline
	}

	if err = h.svc.Update(c.Request().Context(), actor(c), id, patch); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// UpdateItem PATCH /api/bills/:id/items/:itemId
func (h *Bill) UpdateItem(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	itemID, err := parseItemID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req updateItemRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	patch := service.BillItemPatch{HalfDays: req.HalfDays, Amount: req.Amount, Note: req.Note}
	if err = h.svc.UpdateItem(c.Request().Context(), actor(c), id, itemID, patch); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}
