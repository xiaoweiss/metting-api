package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

// AliyunSMSConfig 阿里云短信 SDK 配置
type AliyunSMSConfig struct {
	AccessKeyId     string `json:"accessKeyId"`
	AccessKeySecret string `json:"accessKeySecret"`
	SignName        string `json:"signName"`     // 签名（如"会议室运营"）
	TemplateCode    string `json:"templateCode"` // 模板 code（如 SMS_123456789）
	RegionId        string `json:"regionId,omitempty"`
}

type AliyunSMSSender struct {
	DB *gorm.DB
}

func (s *AliyunSMSSender) Channel() Channel { return ChannelSMS }

// Send 通过阿里云 API 发短信
// 模板参数用 msg.Text 当 JSON 字符串传入（例："{\"name\":\"xxx\",\"date\":\"yyyy-mm-dd\"}"）
// 如果 Text 不是 JSON，fallback 成 {"content": msg.Text}
// 文档：https://help.aliyun.com/document_detail/102364.html
func (s *AliyunSMSSender) Send(ctx context.Context, msg Message) error {
	var cfg AliyunSMSConfig
	enabled, err := LoadConfig(s.DB, ChannelSMS, &cfg)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("阿里云短信未启用")
	}
	if cfg.AccessKeyId == "" || cfg.AccessKeySecret == "" || cfg.SignName == "" || cfg.TemplateCode == "" {
		return errors.New("阿里云短信配置不完整")
	}
	if len(msg.Phones) == 0 {
		return errors.New("短信收件人电话为空")
	}

	// 组装 TemplateParam：
	//   - 若 msg.Text 是 JSON（以 { 开头），作为模板变量直接带上
	//   - 否则视为"模板无变量"，不传 TemplateParam，让阿里云用模板原文发送
	// 这样可以兼容两类模板：有变量的（调用方传 JSON）和纯文本通知类（调用方不关心内容）。
	params := map[string]string{
		"Action":           "SendSms",
		"Version":          "2017-05-25",
		"RegionId":         firstNonEmpty(cfg.RegionId, "cn-hangzhou"),
		"PhoneNumbers":     strings.Join(msg.Phones, ","),
		"SignName":         cfg.SignName,
		"TemplateCode":     cfg.TemplateCode,
		"Format":           "JSON",
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Int()),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"AccessKeyId":      cfg.AccessKeyId,
	}
	if tplParam := strings.TrimSpace(msg.Text); strings.HasPrefix(tplParam, "{") {
		params["TemplateParam"] = tplParam
	}

	// 阿里云 POP 签名：method 影响 stringToSign，这里用 POST。
	// POST 形式下所有业务参数放 body（form-urlencoded）；URL 只保留 host。
	signature := aliSign("POST", params, cfg.AccessKeySecret)
	params["Signature"] = signature

	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	httpClient := &http.Client{Timeout: 8 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://dysmsapi.aliyuncs.com/", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		// 去掉 net/http 默认 err 里拼的超长 URL，只留关键错误类型
		errText := err.Error()
		if i := strings.LastIndex(errText, ": "); i >= 0 {
			errText = errText[i+2:]
		}
		return fmt.Errorf("阿里云短信请求失败: %s", errText)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var r struct {
		Code      string `json:"Code"`
		Message   string `json:"Message"`
		RequestId string `json:"RequestId"`
	}
	_ = json.Unmarshal(body, &r)
	if r.Code != "OK" {
		return fmt.Errorf("阿里云短信发送失败[%s]: %s", r.Code, r.Message)
	}
	logx.Infof("[SMS] 发送成功 RequestId=%s phones=%v", r.RequestId, msg.Phones)
	return nil
}

// aliSign 按阿里云 POP 规范计算签名
func aliSign(method string, params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(aliEscape(k))
		buf.WriteByte('=')
		buf.WriteString(aliEscape(params[k]))
	}
	stringToSign := method + "&" + aliEscape("/") + "&" + aliEscape(buf.String())

	h := hmac.New(sha1.New, []byte(secret+"&"))
	h.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func aliEscape(s string) string {
	return strings.NewReplacer("+", "%20", "*", "%2A", "%7E", "~").Replace(url.QueryEscape(s))
}
