// 一次性调试工具：打印整张表每一行的所有字段（用于确认字段结构）
// 用法：DINGTALK_OPERATOR_ID=xxx DINGTALK_BASE_ID=xxx DINGTALK_WORKSHEET_ID=xxx go run cmd/dumpsheet/main.go
package main

import (
	"encoding/json"
	"fmt"
	"meeting/pkg/dingtalk"
	"os"
)

func main() {
	client := &dingtalk.Client{
		AppKey:    getEnv("DINGTALK_APP_KEY", "dingi0orfgoab5rfhleo"),
		AppSecret: getEnv("DINGTALK_APP_SECRET", "soTdaWwhvYoa9lQ_bNHgJ28LkBqCkEtzoHY1wDIFBmverpKAo4g3hXZe2dQdqELY"),
	}

	operatorId := getEnv("DINGTALK_OPERATOR_ID", "")
	baseId := getEnv("DINGTALK_BASE_ID", "")
	worksheetId := getEnv("DINGTALK_WORKSHEET_ID", "")

	if operatorId == "" || baseId == "" || worksheetId == "" {
		fmt.Println("需要 DINGTALK_OPERATOR_ID / DINGTALK_BASE_ID / DINGTALK_WORKSHEET_ID")
		os.Exit(1)
	}

	sc := &dingtalk.SheetClient{
		Client:      client,
		BaseId:      baseId,
		WorksheetId: worksheetId,
		OperatorId:  operatorId,
	}

	rows, err := sc.GetAllRows()
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}

	allKeys := map[string]bool{}
	for _, r := range rows {
		for k := range r {
			allKeys[k] = true
		}
	}
	fmt.Printf("=== 表格共 %d 行, 字段并集 %d 个 ===\n", len(rows), len(allKeys))
	for k := range allKeys {
		fmt.Printf("  - %s\n", k)
	}
	fmt.Println()

	for i, r := range rows {
		j, _ := json.MarshalIndent(r, "", "  ")
		fmt.Printf("=== 行 %d ===\n%s\n\n", i+1, string(j))
	}
}

func getEnv(k, fb string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fb
}
