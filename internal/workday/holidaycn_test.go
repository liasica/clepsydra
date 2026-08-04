package workday

import "testing"

func TestParseHolidayCN(t *testing.T) {
	data := []byte(`{
		"year": 2026,
		"days": [
			{"name": "元旦", "date": "2026-01-01", "isOffDay": true},
			{"name": "春节调休", "date": "2026-02-15", "isOffDay": false}
		]
	}`)

	entries, err := ParseHolidayCN(data)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("条目数 = %d, want 2", len(entries))
	}
	if entries[0].Type != "holiday" || entries[0].Date != "2026-01-01" {
		t.Errorf("放假日解析错误: %+v", entries[0])
	}
	if entries[1].Type != "workday" || entries[1].Name != "春节调休" {
		t.Errorf("调休日解析错误: %+v", entries[1])
	}

	// 非法 JSON 返回错误
	if _, err = ParseHolidayCN([]byte("not json")); err == nil {
		t.Error("非法 JSON 应返回错误")
	}
}
