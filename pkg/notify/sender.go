// Package notify 多渠道通知发送 —— 钉钉群机器人 / 阿里云短信 / 钉钉工作通知 / 邮件
package notify

import (
	"context"
	"encoding/json"
	"fmt"

	"meeting/internal/model"

	"gorm.io/gorm"
)

// Channel 渠道标识
type Channel string

const (
	ChannelDingTalkRobot Channel = "dingtalk_robot" // 钉钉群自定义机器人（webhook）
	ChannelSMS           Channel = "sms"
	ChannelDingTalkDing  Channel = "dingtalk_ding" // 钉钉工作通知（企业自建应用 asyncsend_v2）
	ChannelEmail         Channel = "email"         // SMTP 邮件（复用 mail_settings 里配置）
)

// Message 统一消息载体
// 各 Sender 按自己渠道的要求自行组装最终 payload
type Message struct {
	Title       string   // 标题（也用作邮件主题）
	Text        string   // 纯文本内容（fallback）
	Markdown    string   // Markdown 内容（钉钉优先用）
	HtmlBody    string   // 邮件正文 HTML，缺省时由 Markdown / Text 自动转
	Phones      []string // 手机号（SMS 用）
	Emails      []string // 邮箱地址（邮件渠道用）
	DingUserIds []string // 钉钉 userId（钉钉 Ding 用）
	Unionids    []string // 钉钉 unionId（钉钉 Ding 可转成 userId 再发）
}

// Sender 渠道发送接口
type Sender interface {
	Channel() Channel
	Send(ctx context.Context, msg Message) error
}

// firstNonEmpty 返回第一个非空字符串
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// LoadConfig 从 DB 读某渠道配置 JSON → 解到 out
func LoadConfig(db *gorm.DB, channel Channel, out interface{}) (enabled bool, err error) {
	var row model.NotificationSetting
	if err := db.Where("channel = ?", string(channel)).First(&row).Error; err != nil {
		return false, fmt.Errorf("渠道 %s 未配置: %w", channel, err)
	}
	if row.Config == "" {
		return row.Enabled, nil
	}
	if err := json.Unmarshal([]byte(row.Config), out); err != nil {
		return row.Enabled, fmt.Errorf("渠道 %s 配置 JSON 解析失败: %w", channel, err)
	}
	return row.Enabled, nil
}
