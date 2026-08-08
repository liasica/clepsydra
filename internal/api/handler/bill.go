package handler

import (
	"strconv"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
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

// generateRequest 生成账单请求体
type generateRequest struct {
	Period string `json:"period"`
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

	return api.OK(c, newBillDetailDTO(b))
}

// Generate POST /api/bills/generate
func (h *Bill) Generate(c echo.Context) error {
	var req generateRequest
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	b, err := h.svc.Generate(c.Request().Context(), actor(c), req.Period)
	if err != nil {
		return api.Fail(c, err)
	}

	full, err := h.svc.Get(c.Request().Context(), b.ID)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, newBillDetailDTO(full))
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
