package dingtalk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// SheetRow 表格每行数据，key 为列名，value 为单元格值
type SheetRow map[string]interface{}

// SheetRecord 包含 recordId 的行数据（用于 linkedRecordId 解析）
type SheetRecord struct {
	RecordId string
	Fields   SheetRow
}

// WorksheetInfo 工作表基本信息
type WorksheetInfo struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// SheetClient 多维表格客户端（Notable API）
type SheetClient struct {
	Client      *Client // 复用企业应用 client 获取 token
	BaseId      string  // 多维表格文档 ID（URL 中的 nodeId）
	WorksheetId string  // 工作表 ID
	OperatorId  string  // 操作人 unionId（必填）
}

// ListWorksheets 列出指定文档下所有工作表
func (s *SheetClient) ListWorksheets() ([]WorksheetInfo, error) {
	token, err := s.Client.GetCorpToken()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.dingtalk.com/v1.0/notable/bases/%s/sheets?operatorId=%s",
		s.BaseId, s.OperatorId)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("x-acs-dingtalk-access-token", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// 先尝试直接解析为数组（新版 API 直接返回数组）
	var sheets []WorksheetInfo
	if err := json.Unmarshal(body, &sheets); err == nil && len(sheets) > 0 {
		return sheets, nil
	}

	// 兜底：尝试解析为 { value: [...] } 格式
	var wrapped struct {
		Value []WorksheetInfo `json:"value"`
		Code  string          `json:"code"`
		Msg   string          `json:"message"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w\n原始响应: %s", err, string(body))
	}
	if wrapped.Code != "" {
		return nil, fmt.Errorf("API 错误 [%s]: %s\n原始响应: %s", wrapped.Code, wrapped.Msg, string(body))
	}
	return wrapped.Value, nil
}

// WithWorksheet 返回指向不同工作表的 SheetClient 副本
func (s *SheetClient) WithWorksheet(worksheetId string) *SheetClient {
	return &SheetClient{
		Client:      s.Client,
		BaseId:      s.BaseId,
		WorksheetId: worksheetId,
		OperatorId:  s.OperatorId,
	}
}

// GetAllRecords 拉取所有行并保留 recordId（用于 linkedRecordId 解析）
func (s *SheetClient) GetAllRecords() ([]SheetRecord, error) {
	token, err := s.Client.GetCorpToken()
	if err != nil {
		return nil, err
	}

	var allRecords []SheetRecord
	nextToken := ""
	for {
		records, next, err := s.fetchPageRecords(token, nextToken)
		if err != nil {
			return nil, err
		}
		allRecords = append(allRecords, records...)
		if next == "" {
			break
		}
		nextToken = next
	}
	return allRecords, nil
}

func (s *SheetClient) fetchPageRecords(token, pageToken string) ([]SheetRecord, string, error) {
	url := fmt.Sprintf(
		"https://api.dingtalk.com/v1.0/notable/bases/%s/sheets/%s/records/list?operatorId=%s",
		s.BaseId, s.WorksheetId, s.OperatorId,
	)

	payload := map[string]interface{}{
		"maxResults": 100,
	}
	if pageToken != "" {
		payload["nextToken"] = pageToken
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("x-acs-dingtalk-access-token", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Records []struct {
			Id     string   `json:"id"`
			Fields SheetRow `json:"fields"`
		} `json:"records"`
		HasMore   bool   `json:"hasMore"`
		NextToken string `json:"nextToken"`
		Code      string `json:"code"`
		Message   string `json:"message"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, "", fmt.Errorf("解析响应失败: %w\n原始: %s", err, string(respBody))
	}
	if result.Code != "" {
		return nil, "", fmt.Errorf("API 错误 [%s]: %s", result.Code, result.Message)
	}

	var records []SheetRecord
	for _, r := range result.Records {
		records = append(records, SheetRecord{RecordId: r.Id, Fields: r.Fields})
	}

	nextToken := ""
	if result.HasMore {
		nextToken = result.NextToken
	}
	return records, nextToken, nil
}

// GetAllRows 拉取工作表所有行（自动分页）
func (s *SheetClient) GetAllRows() ([]SheetRow, error) {
	token, err := s.Client.GetCorpToken()
	if err != nil {
		return nil, err
	}

	var allRows []SheetRow
	nextToken := ""

	for {
		rows, next, err := s.fetchPage(token, nextToken)
		if err != nil {
			return nil, err
		}
		allRows = append(allRows, rows...)
		if next == "" {
			break
		}
		nextToken = next
	}

	return allRows, nil
}

func (s *SheetClient) fetchPage(token, pageToken string) ([]SheetRow, string, error) {
	url := fmt.Sprintf(
		"https://api.dingtalk.com/v1.0/notable/bases/%s/sheets/%s/records/list?operatorId=%s",
		s.BaseId, s.WorksheetId, s.OperatorId,
	)

	payload := map[string]interface{}{
		"maxResults": 100,
	}
	if pageToken != "" {
		payload["nextToken"] = pageToken
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("x-acs-dingtalk-access-token", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Records []struct {
			Id     string   `json:"id"`
			Fields SheetRow `json:"fields"`
		} `json:"records"`
		HasMore   bool   `json:"hasMore"`
		NextToken string `json:"nextToken"`
		// 错误响应
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, "", fmt.Errorf("解析表格响应失败: %w\n原始响应: %s", err, string(respBody))
	}
	if result.Code != "" {
		return nil, "", fmt.Errorf("钉钉表格 API 错误 [%s]: %s\n原始响应: %s", result.Code, result.Message, string(respBody))
	}

	var rows []SheetRow
	for _, r := range result.Records {
		rows = append(rows, r.Fields)
	}

	nextToken := ""
	if result.HasMore {
		nextToken = result.NextToken
	}

	return rows, nextToken, nil
}

// PrintColumns 打印第一行的所有列名（用于调试确认列名映射）
func (s *SheetClient) PrintColumns() error {
	rows, err := s.GetAllRows()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Println("表格为空（无数据行）")
		return nil
	}
	fmt.Println("=== 钉钉表格列名 ===")
	for col, val := range rows[0] {
		fmt.Printf("  列名: %-30s 示例值: %v\n", col, val)
	}
	fmt.Printf("=== 共 %d 行 ===\n", len(rows))
	return nil
}
