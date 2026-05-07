package sync

import (
	"math"
	"strconv"
	"strings"
	"time"

	"meeting/pkg/dingtalk"
)

// textField 提取文本字段
// 兼容钉钉把"看似文本"的字段返回成单选/link 格式 {id, name}
// 例如 Daily Data Input 里「酒店名称 Hotel Name」是 lookup，返回 map；
// 而 酒店基础信息表 里同名字段是纯字符串
func textField(row dingtalk.SheetRow, key string) string {
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if m, ok := v.(map[string]interface{}); ok {
		if name, ok := m["name"].(string); ok {
			return name
		}
	}
	return ""
}

// singleSelectName 提取单选字段的 name
// API 格式: {"id": "xxx", "name": "值"}
func singleSelectName(row dingtalk.SheetRow, key string) string {
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return ""
	}
	name, _ := m["name"].(string)
	return name
}

// multipleSelectNames 提取多选字段的所有 name
// API 格式: [{"id": "xxx", "name": "上午"}, {"id": "xxx", "name": "下午"}]
func multipleSelectNames(row dingtalk.SheetRow, key string) []string {
	v, ok := row[key]
	if !ok || v == nil {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var names []string
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := m["name"].(string); ok {
			names = append(names, name)
		}
	}
	return names
}

// linkedRecordId 提取单值关联字段的 recordId
// 兼容两种 API 格式：
//   - {"id": "xxx", "name": "yyy"}                  (Daily Data 里 Room Name / Hotel Name)
//   - {"linkedRecordIds": ["xxx"], "name": "yyy"}   (酒店会议室信息表里「选择酒店」)
func linkedRecordId(row dingtalk.SheetRow, key string) string {
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return ""
	}
	if id, ok := m["id"].(string); ok && id != "" {
		return id
	}
	if ids, ok := m["linkedRecordIds"].([]interface{}); ok {
		for _, x := range ids {
			if s, ok := x.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// linkedRecordIds 提取关联字段的 recordId 列表
// API 格式: {"linkedRecordIds": ["abc", "def"]}
func linkedRecordIds(row dingtalk.SheetRow, key string) []string {
	v, ok := row[key]
	if !ok || v == nil {
		return nil
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	ids, ok := m["linkedRecordIds"].([]interface{})
	if !ok {
		return nil
	}
	var result []string
	for _, id := range ids {
		if s, ok := id.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// checkboxField 提取勾选框字段
func checkboxField(row dingtalk.SheetRow, key string) bool {
	v, ok := row[key]
	if !ok || v == nil {
		return false
	}
	b, _ := v.(bool)
	return b
}

// dateField 提取日期字段（钉钉返回 unix 毫秒时间戳）
func dateField(row dingtalk.SheetRow, key string) *time.Time {
	v, ok := row[key]
	if !ok || v == nil {
		return nil
	}
	f, ok := v.(float64)
	if !ok || f == 0 {
		return nil
	}
	t := time.UnixMilli(int64(f))
	return &t
}

// numberField 提取数字字段，NaN/Inf 返回 0
// 兼容钉钉把数字存成字符串（如 剧院式 Theater = "50"）
func numberField(row dingtalk.SheetRow, key string) float64 {
	v, ok := row[key]
	if !ok || v == nil {
		return 0
	}
	if f, ok := v.(float64); ok {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0
		}
		return f
	}
	if s, ok := v.(string); ok {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return 0
}

type userInfo struct {
	UnionId string
	Name    string
}

// userFields 提取人员字段
// API 格式: [{"unionId": "xxx", "name": "钱缘"}]
func userFields(row dingtalk.SheetRow, key string) []userInfo {
	v, ok := row[key]
	if !ok || v == nil {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var users []userInfo
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		u := userInfo{}
		u.UnionId, _ = m["unionId"].(string)
		u.Name, _ = m["name"].(string)
		if u.UnionId != "" {
			users = append(users, u)
		}
	}
	return users
}

// mapPeriod 将中文时段名转为 DB enum
func mapPeriod(s string) string {
	switch {
	case strings.Contains(s, "上午") || strings.Contains(s, "AM"):
		return "AM"
	case strings.Contains(s, "下午") || strings.Contains(s, "PM"):
		return "PM"
	case strings.Contains(s, "晚上") || strings.Contains(s, "EV"):
		return "EV"
	default:
		return s
	}
}
