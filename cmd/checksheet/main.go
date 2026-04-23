// 调试工具：用免登 code 获取 unionId + 发现钉钉多维表格结构
//
// 用法：
//
// 第一步：用免登 code 获取你的 unionId
//   DINGTALK_AUTH_CODE=xxx go run cmd/checksheet/main.go
//
// 第二步：列出所有工作表
//   DINGTALK_OPERATOR_ID=xxx DINGTALK_BASE_ID=xxx go run cmd/checksheet/main.go
//
// 第三步：确认列名
//   DINGTALK_OPERATOR_ID=xxx DINGTALK_BASE_ID=xxx DINGTALK_WORKSHEET_ID=xxx go run cmd/checksheet/main.go
package main

import (
	"fmt"
	"meeting/pkg/dingtalk"
	"os"
)

func main() {
	appKey := getEnv("DINGTALK_APP_KEY", "dingi0orfgoab5rfhleo")
	appSecret := getEnv("DINGTALK_APP_SECRET", "soTdaWwhvYoa9lQ_bNHgJ28LkBqCkEtzoHY1wDIFBmverpKAo4g3hXZe2dQdqELY")
	authCode := getEnv("DINGTALK_AUTH_CODE", "")
	operatorId := getEnv("DINGTALK_OPERATOR_ID", "")
	baseId := getEnv("DINGTALK_BASE_ID", "")
	worksheetId := getEnv("DINGTALK_WORKSHEET_ID", "")

	client := &dingtalk.Client{
		AppKey:    appKey,
		AppSecret: appSecret,
	}

	// ========== 模式一：用 auth code 换取 unionId ==========
	if authCode != "" {
		fmt.Println("正在用免登 code 获取用户信息...")
		userInfo, err := client.GetUserByCode(authCode)
		if err != nil {
			fmt.Printf("❌ 获取用户信息失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ 用户信息获取成功：")
		fmt.Printf("  姓名:    %s\n", userInfo.Name)
		fmt.Printf("  UnionId: %s\n", userInfo.UnionId)
		fmt.Printf("  Email:   %s\n", userInfo.Email)
		fmt.Println("")
		fmt.Println("请将 UnionId 填入 etc/meeting-api.yaml 的 DingTalk.Sheet.OperatorId")
		fmt.Println("然后测试表格读取：")
		fmt.Printf("  DINGTALK_OPERATOR_ID=%s DINGTALK_BASE_ID=你的BaseId go run cmd/checksheet/main.go\n", userInfo.UnionId)
		return
	}

	// ========== 模式二：读取表格（需要 operatorId） ==========
	if operatorId == "" {
		fmt.Println("⚠️  需要先获取 operatorId（你的 unionId）")
		fmt.Println("")
		fmt.Println("方法：在钉钉 JSAPI Explorer 中调试 requestAuthCode 获取 code，然后运行：")
		fmt.Println("  DINGTALK_AUTH_CODE=你的code go run cmd/checksheet/main.go")
		os.Exit(0)
	}

	// 验证 token
	fmt.Println("正在获取钉钉 AccessToken...")
	token, err := client.GetCorpToken()
	if err != nil {
		fmt.Printf("❌ 获取 Token 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Token 获取成功（前20字符）: %s...\n\n", token[:20])

	if baseId == "" {
		fmt.Println("⚠️  未设置 DINGTALK_BASE_ID")
		fmt.Println("请从多维表格文档 URL 获取 BaseId：")
		fmt.Println("  例如：https://alidocs.dingtalk.com/i/nodes/{这里就是BaseId}")
		fmt.Println("")
		fmt.Printf("  DINGTALK_OPERATOR_ID=%s DINGTALK_BASE_ID=你的BaseId go run cmd/checksheet/main.go\n", operatorId)
		os.Exit(0)
	}

	sc := &dingtalk.SheetClient{
		Client:      client,
		BaseId:      baseId,
		WorksheetId: worksheetId,
		OperatorId:  operatorId,
	}

	// 列出所有工作表
	fmt.Printf("正在获取文档 [%s] 的工作表列表...\n", baseId)
	sheets, err := sc.ListWorksheets()
	if err != nil {
		fmt.Printf("❌ 获取工作表列表失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ 共找到 %d 个工作表：\n", len(sheets))
	fmt.Println("┌─────────────────────────────────┬──────────────────────┐")
	fmt.Printf("│ %-31s │ %-20s │\n", "工作表名称", "WorksheetId")
	fmt.Println("├─────────────────────────────────┼──────────────────────┤")
	for _, s := range sheets {
		fmt.Printf("│ %-31s │ %-20s │\n", s.Name, s.Id)
	}
	fmt.Println("└─────────────────────────────────┴──────────────────────┘")

	if worksheetId == "" {
		fmt.Println("\n请将目标 WorksheetId 填入 etc/meeting-api.yaml 的 DingTalk.Sheet.WorksheetId")
		fmt.Println("然后重新运行确认列名：")
		fmt.Printf("  DINGTALK_OPERATOR_ID=%s DINGTALK_BASE_ID=%s DINGTALK_WORKSHEET_ID=xxx go run cmd/checksheet/main.go\n",
			operatorId, baseId)
		os.Exit(0)
	}

	// 已有 WorksheetId，打印列名
	fmt.Printf("\n正在读取工作表 [%s] 的列名...\n", worksheetId)
	if err := sc.PrintColumns(); err != nil {
		fmt.Printf("❌ 读取失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("\n✅ 请将上面的列名填入 etc/meeting-api.yaml 的 DingTalk.Sheet.Mapping 配置中")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
