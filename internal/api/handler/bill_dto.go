package handler

import (
	"time"

	"clepsydra/internal/ent"
)

// billDTO 账单响应结构，字段与 openapi.yaml 的 Bill schema 逐一对齐
// 显式声明 JSON tag 且不带 omitempty，避免 ent 生成结构体的零值字段被省略
type billDTO struct {
	ID              int           `json:"id"`
	Name            string        `json:"name"`
	Period          *string       `json:"period"`
	Status          string        `json:"status"`
	DailyRate       int           `json:"daily_rate"`
	BaseFee         int           `json:"base_fee"`
	TotalHalfDays   int           `json:"total_half_days"`
	TotalAmount     int           `json:"total_amount"`
	ConfirmDeadline *time.Time    `json:"confirm_deadline"`
	ConfirmedAt     *time.Time    `json:"confirmed_at"`
	ConfirmedBy     *int          `json:"confirmed_by"`
	ConfirmAuto     bool          `json:"confirm_auto"`
	PaidAt          *time.Time    `json:"paid_at"`
	PaidBy          *int          `json:"paid_by"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
	Items           []billItemDTO `json:"items,omitempty"` // 仅账单详情接口填充，列表接口保持缺省
}

// billItemDTO 账单明细响应结构，字段与 openapi.yaml 的 BillItem schema 逐一对齐
type billItemDTO struct {
	ID               int        `json:"id"`
	DemandID         int        `json:"demand_id"`
	DemandTitle      string     `json:"demand_title"`
	DemandStatus     string     `json:"demand_status"`
	HalfDays         int        `json:"half_days"`
	Amount           int        `json:"amount"`
	Billable         bool       `json:"billable"`
	Waived           bool       `json:"waived"`
	PlannedStartDate *time.Time `json:"planned_start_date"`
	Note             string     `json:"note"`
	CreatedAt        time.Time  `json:"created_at"`
}

// newBillDTO 将 ent.Bill 映射为不含明细的响应结构，供列表接口使用
func newBillDTO(b *ent.Bill) billDTO {
	return billDTO{
		ID:              b.ID,
		Name:            b.Name,
		Period:          b.Period,
		Status:          b.Status.String(),
		DailyRate:       b.DailyRate,
		BaseFee:         b.BaseFee,
		TotalHalfDays:   b.TotalHalfDays,
		TotalAmount:     b.TotalAmount,
		ConfirmDeadline: b.ConfirmDeadline,
		ConfirmedAt:     b.ConfirmedAt,
		ConfirmedBy:     b.ConfirmedBy,
		ConfirmAuto:     b.ConfirmAuto,
		PaidAt:          b.PaidAt,
		PaidBy:          b.PaidBy,
		CreatedAt:       b.CreatedAt,
		UpdatedAt:       b.UpdatedAt,
	}
}

// newBillItemDTO 将 ent.BillItem 映射为响应结构
func newBillItemDTO(it *ent.BillItem) billItemDTO {
	return billItemDTO{
		ID:               it.ID,
		DemandID:         it.DemandID,
		DemandTitle:      it.DemandTitle,
		DemandStatus:     it.DemandStatus,
		HalfDays:         it.HalfDays,
		Amount:           it.Amount,
		Billable:         it.Billable,
		Waived:           it.Waived,
		PlannedStartDate: it.PlannedStartDate,
		Note:             it.Note,
		CreatedAt:        it.CreatedAt,
	}
}

// newBillDetailDTO 将 ent.Bill 映射为含顶层 items 的详情响应结构
// 明细取自 b.Edges.Items，调用方需确保查询时已 WithItems 预加载
func newBillDetailDTO(b *ent.Bill) billDTO {
	dto := newBillDTO(b)

	items := make([]billItemDTO, 0, len(b.Edges.Items))
	for _, it := range b.Edges.Items {
		items = append(items, newBillItemDTO(it))
	}
	dto.Items = items

	return dto
}
