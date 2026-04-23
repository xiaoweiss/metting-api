package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// DingTalkRobotConfig 钉钉群自定义机器人配置
//   - WebhookUrl: 完整 URL（含 access_token），形如
//     https://oapi.dingtalk.com/robot/send?access_token=xxx
//   - Secret: 加签模式的 Secret（SEC 开头），可选；没开加签时留空
type DingTalkRobotConfig struct {
	WebhookUrl string `json:"webhookUrl"`
	Secret     string `json:"secret,omitempty"`
}

type DingTalkRobotSender struct {
	DB *gorm.DB
}

func (s *DingTalkRobotSender) Channel() Channel { return ChannelDingTalkRobot }

// Send 向钉钉群自定义机器人发送消息（markdown 优先，否则 text）
// 文档：https://open.dingtalk.com/document/robots/custom-robot-access
func (s *DingTalkRobotSender) Send(ctx context.Context, msg Message) error {
	var cfg DingTalkRobotConfig
	enabled, err := LoadConfig(s.DB, ChannelDingTalkRobot, &cfg)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("钉钉机器人未启用")
	}
	if cfg.WebhookUrl == "" {
		return errors.New("钉钉机器人 webhookUrl 未配置")
	}

	content := firstNonEmpty(msg.Markdown, msg.Text)
	if content == "" {
		return errors.New("消息内容为空")
	}

	var payload map[string]interface{}
	if msg.Markdown != "" {
		payload = map[string]interface{}{
			"msgtype": "markdown",
			"markdown": map[string]string{
				"title": firstNonEmpty(msg.Title, "通知"),
				"text":  content,
			},
		}
	} else {
		payload = map[string]interface{}{
			"msgtype": "text",
			"text":    map[string]string{"content": content},
		}
	}

	body, _ := json.Marshal(payload)

	// 钉钉加签：timestamp 用毫秒；sign 进 URL query
	finalURL := cfg.WebhookUrl
	if cfg.Secret != "" {
		ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
		sign := dingTalkRobotSign(ts, cfg.Secret)
		sep := "&"
		if !strings.Contains(finalURL, "?") {
			sep = "?"
		}
		finalURL = fmt.Sprintf("%s%stimestamp=%s&sign=%s", finalURL, sep, ts, url.QueryEscape(sign))
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", finalURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	rb, _ := io.ReadAll(resp.Body)
	var r struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	_ = json.Unmarshal(rb, &r)
	if r.Errcode != 0 {
		return fmt.Errorf("钉钉机器人发送失败[%d]: %s", r.Errcode, r.Errmsg)
	}
	return nil
}

// dingTalkRobotSign 生成加签字符串
//
//	stringToSign = timestamp + "\n" + secret
//	sign         = base64( HmacSHA256(stringToSign, secret) )
func dingTalkRobotSign(timestamp, secret string) string {
	stringToSign := timestamp + "\n" + secret
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
