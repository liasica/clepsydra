package workday

import "encoding/json"

// holidayCNFile holiday-cn（github.com/NateScarlet/holiday-cn）年度数据文件结构
type holidayCNFile struct {
	Year int `json:"year"`
	Days []struct {
		Name     string `json:"name"`
		Date     string `json:"date"`
		IsOffDay bool   `json:"isOffDay"`
	} `json:"days"`
}

// ParseHolidayCN 解析 holiday-cn 年度 JSON 为节假日条目
// isOffDay 为 true 表示放假，false 表示调休补班
func ParseHolidayCN(data []byte) ([]Entry, error) {
	var file holidayCNFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, len(file.Days))
	for _, d := range file.Days {
		entryType := "workday"
		if d.IsOffDay {
			entryType = "holiday"
		}
		entries = append(entries, Entry{Date: d.Date, Type: entryType, Name: d.Name})
	}

	return entries, nil
}
