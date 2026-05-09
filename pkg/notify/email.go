package notify

import (
	"context"
	"errors"
	"html"
	"strings"

	"gorm.io/gorm"

	"meeting/internal/config"
	"meeting/pkg/mail"
)

// EmailSender 通过 SMTP 发邮件，复用后台已配置的 mail_settings。
// 不需要 notification_settings.config 里再写一份 SMTP 凭证 ——
// LoadConfig 只用来读 enabled 开关。
type EmailSender struct {
	DB  *gorm.DB
	Cfg config.Config
}

func (s *EmailSender) Channel() Channel { return ChannelEmail }

func (s *EmailSender) Send(ctx context.Context, msg Message) error {
	// 仅检查渠道开关；SMTP 凭证由 mail.Sender.LoadSettings 从 mail_settings / yaml 读
	var dummy struct{}
	enabled, err := LoadConfig(s.DB, ChannelEmail, &dummy)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("邮件渠道未启用")
	}
	if len(msg.Emails) == 0 {
		return errors.New("邮件收件人列表为空")
	}

	subject := strings.TrimSpace(msg.Title)
	if subject == "" {
		subject = "系统通知"
	}

	body := buildEmailHtml(msg)
	mailer := mail.NewSender(s.DB, s.Cfg)
	return mailer.Send(msg.Emails, subject, body, nil)
}

// buildEmailHtml 优先用 HtmlBody；没有则把 Markdown 简单转 HTML（保留换行 + bullet）；
// 都没有就 escape 一下 Text 包到 <pre>。
// 这里不引第三方 markdown 库，只处理 update_check 用到的那点格式（**bold**、•、换行）。
func buildEmailHtml(msg Message) string {
	if strings.TrimSpace(msg.HtmlBody) != "" {
		return msg.HtmlBody
	}
	source := msg.Markdown
	if strings.TrimSpace(source) == "" {
		source = msg.Text
	}
	if strings.TrimSpace(source) == "" {
		return ""
	}
	escaped := html.EscapeString(source)
	// **xxx** → <strong>xxx</strong>
	for {
		i := strings.Index(escaped, "**")
		if i < 0 {
			break
		}
		j := strings.Index(escaped[i+2:], "**")
		if j < 0 {
			break
		}
		escaped = escaped[:i] + "<strong>" + escaped[i+2:i+2+j] + "</strong>" + escaped[i+2+j+2:]
	}
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	return `<div style="font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;font-size:14px;line-height:1.6;color:#1f2937;">` + escaped + `</div>`
}
