package mail

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"strings"

	"gopkg.in/gomail.v2"
	"gorm.io/gorm"

	"meeting/internal/config"
	"meeting/internal/model"
)

type Sender struct {
	DB     *gorm.DB
	Config config.Config
}

func NewSender(db *gorm.DB, c config.Config) *Sender {
	return &Sender{DB: db, Config: c}
}

type Settings struct {
	Host     string
	Port     int
	Username string
	Password string
	FromName string
}

// LoadSettings 优先读取 DB 中保存的 SMTP 配置，若没有则回退到 yaml。
func (s *Sender) LoadSettings() (*Settings, error) {
	var m model.MailSetting
	if err := s.DB.First(&m).Error; err == nil && m.SmtpHost != "" {
		return &Settings{
			Host:     m.SmtpHost,
			Port:     m.SmtpPort,
			Username: m.Username,
			Password: m.Password,
			FromName: m.FromName,
		}, nil
	}
	if s.Config.Mail.Host == "" || s.Config.Mail.Username == "" {
		return nil, errors.New("SMTP 配置未设置，请先在后台完成发件配置")
	}
	return &Settings{
		Host:     s.Config.Mail.Host,
		Port:     s.Config.Mail.Port,
		Username: s.Config.Mail.Username,
		Password: s.Config.Mail.Password,
		FromName: s.Config.Mail.FromName,
	}, nil
}

// InlineImage 邮件正文 inline 嵌入的图片。
// gomail.Embed 自动把 Content-ID 设为文件 basename,HTML 引用 <img src="cid:basename"> 即可。
type InlineImage struct {
	FilePath string // 本地绝对路径
}

// Attachment 邮件附件(非 inline),收件人在客户端附件区看到。
type Attachment struct {
	FilePath string // 本地绝对路径
	Filename string // 在邮件里显示的文件名(默认 = basename)
}

// Send 发送一封邮件。to 可以是多个地址。
// inlineImages 用 gomail.Embed 嵌入(Content-ID = filename basename),HTML 可 <img src="cid:..."> 引用。
// attachments 用 gomail.Attach 当附件,Content-Disposition: attachment。
func (s *Sender) Send(to []string, subject, htmlBody string, inlineImages []InlineImage, attachments []Attachment) error {
	if len(to) == 0 {
		return errors.New("收件人为空")
	}
	settings, err := s.LoadSettings()
	if err != nil {
		return err
	}
	if settings.Password == "" {
		return errors.New("SMTP 密码为空，请在后台填写授权码/密码")
	}

	m := gomail.NewMessage()
	from := settings.Username
	if settings.FromName != "" {
		m.SetAddressHeader("From", settings.Username, settings.FromName)
	} else {
		m.SetHeader("From", from)
	}
	m.SetHeader("To", to...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", htmlBody)
	for _, img := range inlineImages {
		m.Embed(img.FilePath)
	}
	for _, att := range attachments {
		if att.Filename != "" && att.Filename != att.FilePath {
			m.Attach(att.FilePath, gomail.Rename(att.Filename))
		} else {
			m.Attach(att.FilePath)
		}
	}

	d := gomail.NewDialer(settings.Host, settings.Port, settings.Username, settings.Password)
	// 465 端口通常使用 SSL
	if settings.Port == 465 {
		d.SSL = true
	}
	return d.DialAndSend(m)
}

// RenderTemplate 使用 Go 的 html/template 渲染模板；vars 是变量键值。
func RenderTemplate(tpl string, vars map[string]interface{}) (string, error) {
	t, err := template.New("email").Parse(tpl)
	if err != nil {
		return "", fmt.Errorf("模板解析失败: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("模板渲染失败: %w", err)
	}
	return buf.String(), nil
}

// RenderSubjectAndBody 同时渲染 subject 和 body。
func RenderSubjectAndBody(subjectTpl, bodyTpl string, vars map[string]interface{}) (string, string, error) {
	subject, err := RenderTemplate(subjectTpl, vars)
	if err != nil {
		return "", "", err
	}
	// 让 subject 不含换行
	subject = strings.TrimSpace(strings.ReplaceAll(subject, "\n", " "))
	body, err := RenderTemplate(bodyTpl, vars)
	if err != nil {
		return "", "", err
	}
	return subject, body, nil
}
