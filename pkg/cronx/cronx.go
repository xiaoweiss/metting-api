// Package cronx 包装 robfig/cron 让所有 scheduler 都支持秒级表达式，
// 同时兼容历史的 5 字段表达式（自动 prefix "0 " 提升到 6 字段）。
package cronx

import (
	"fmt"
	"strings"

	"github.com/robfig/cron/v3"
)

// New 返回一个支持秒级精度的 cron.Cron。
// 6 字段格式: 秒 分 时 日 月 周
func New() *cron.Cron {
	return cron.New(cron.WithSeconds())
}

// Normalize 把 5 字段表达式补齐成 6 字段（秒数补 0），
// 6 字段表达式原样返回，其它长度报错。
// 例：
//
//	"0 20 * * *"      -> "0 0 20 * * *"     (每天 20:00:00)
//	"*/30 * * * * *"  -> "*/30 * * * * *"   (每 30 秒)
//	"0 0 20 * * *"    -> "0 0 20 * * *"     (原样)
func Normalize(expr string) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", fmt.Errorf("cron 表达式为空")
	}
	parts := strings.Fields(expr)
	switch len(parts) {
	case 5:
		return "0 " + expr, nil
	case 6:
		return expr, nil
	default:
		return "", fmt.Errorf("cron 表达式字段数不对（应为 5 或 6 段，实际 %d）", len(parts))
	}
}
