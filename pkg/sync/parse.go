package sync

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"meeting/pkg/dingtalk"
)

// normColumnKey 归一化列名:只保留中文 + 英文字母 + 数字,去掉空格/括号/√ 等标点。
// 钉钉经常给列名加英文后缀或改中英文间空格(如「酒店名称 Hotel Name」→「酒店名称Hotel Name」、
// 「会议室类型 （Meeting Room Category)」→「会议室类型Meeting Room Category」),归一后能稳定匹配。
func normColumnKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// resolveKey 在 row 里找跟 want 对应的实际列名,容忍钉钉列名加后缀/改空格/改标点:
//  1. 精确匹配
//  2. 归一化后精确匹配(吸收「去空格/去括号」类变化)
//  3. 归一化后前缀匹配(吸收「中文不变、尾部加英文翻译」类变化)
//
// 找不到则返回 want 原值(让下游按缺字段处理)。
func resolveKey(row dingtalk.SheetRow, want string) string {
	if _, ok := row[want]; ok {
		return want
	}
	nw := normColumnKey(want)
	if nw == "" {
		return want
	}
	for k := range row {
		if normColumnKey(k) == nw {
			return k
		}
	}
	for k := range row {
		if strings.HasPrefix(normColumnKey(k), nw) {
			return k
		}
	}
	return want
}

// textField 提取文本字段
// 兼容钉钉把"看似文本"的字段返回成单选/link 格式 {id, name}
func textField(row dingtalk.SheetRow, key string) string {
	key = resolveKey(row, key)
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
	key = resolveKey(row, key)
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
	key = resolveKey(row, key)
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
	key = resolveKey(row, key)
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

// linkedRecordName 提取单值关联字段的 name 部分
// 兼容两种 API 格式:
//   - {"id": "xxx", "name": "yyy"}                  (Daily Data 里 Room Name / Hotel Name)
//   - {"linkedRecordIds": ["xxx"], "name": "yyy"}   (酒店会议室信息表里「选择酒店」)
func linkedRecordName(row dingtalk.SheetRow, key string) string {
	key = resolveKey(row, key)
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return ""
	}
	if name, ok := m["name"].(string); ok && name != "" {
		return name
	}
	return ""
}

// linkedRecordIds 提取关联字段的 recordId 列表
// API 格式: {"linkedRecordIds": ["abc", "def"]}
func linkedRecordIds(row dingtalk.SheetRow, key string) []string {
	key = resolveKey(row, key)
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
	key = resolveKey(row, key)
	v, ok := row[key]
	if !ok || v == nil {
		return false
	}
	b, _ := v.(bool)
	return b
}

// dateField 提取日期字段（钉钉返回 unix 毫秒时间戳）
func dateField(row dingtalk.SheetRow, key string) *time.Time {
	key = resolveKey(row, key)
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
	key = resolveKey(row, key)
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
	key = resolveKey(row, key)
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
