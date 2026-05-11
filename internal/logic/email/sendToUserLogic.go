package email

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"
	"meeting/pkg/blast"
	pkgmail "meeting/pkg/mail"

	"github.com/zeromicro/go-zero/core/logx"
)

type SendToUserLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSendToUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendToUserLogic {
	return &SendToUserLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// SendToUser 给指定用户/邮箱发送一封测试邮件。
// 支持两种方式：
//  1. 传 userId，从 users 表取邮箱。
//  2. 传 email（可选 name），直接发给该邮箱（用于同步进来的非系统用户）。
func (l *SendToUserLogic) SendToUser(req *types.SendToUserReq) (resp *types.BaseResp, err error) {
	var userName, userEmail string

	if req.UserId > 0 {
		var u model.User
		if err := l.svcCtx.DB.Select("id, name, email").Where("id = ?", req.UserId).First(&u).Error; err != nil {
			return nil, errors.New("用户不存在")
		}
		userName = u.Name
		userEmail = u.Email
	} else if req.Email != "" {
		userEmail = req.Email
		userName = req.Name
	} else {
		return nil, errors.New("需要提供 userId 或 email")
	}
	if userEmail == "" {
		return nil, errors.New("收件邮箱为空，无法发送")
	}
	if userName == "" {
		userName = userEmail
	}
	u := struct {
		Name  string
		Email string
	}{Name: userName, Email: userEmail}

	var (
		subject, body string
		inlineImages  []pkgmail.InlineImage
		attachments   []pkgmail.Attachment
	)
	if req.TemplateName != "" {
		var tpl model.MailTemplate
		if err := l.svcCtx.DB.Where("name = ?", req.TemplateName).First(&tpl).Error; err != nil {
			return nil, errors.New("模板不存在: " + req.TemplateName)
		}
		// 跟群发链路一致:走 RecipientVars,自动按对标酒店当日数据 +
		// DashboardImage(PNG inline) + DashboardPDF(PDF 附件) 装配
		vars, inl, att, _, _, _ := blast.RecipientVars(l.svcCtx.DB, l.svcCtx.Config, userEmail, time.Now(), req.HotelId)
		// 测试发送时,UserName 兜底
		if v, _ := vars["UserName"].(string); v == "" || v == userEmail {
			vars["UserName"] = u.Name
		}
		inlineImages = inl
		attachments = att
		subject, body, err = pkgmail.RenderSubjectAndBody(tpl.Subject, tpl.Body, vars)
		if err != nil {
			return nil, err
		}

		// 加载模板级静态附件(图 inline + 其它 attach)
		var atts []model.MailTemplateAttachment
		l.svcCtx.DB.Where("template_id = ?", tpl.Id).Order("sort_order, id").Find(&atts)
		for _, a := range atts {
			abs := filepath.Join(l.svcCtx.Config.Mail.AttachmentDir, a.FilePath)
			if strings.HasPrefix(a.MimeType, "image/") {
				inlineImages = append(inlineImages, pkgmail.InlineImage{FilePath: abs})
			} else {
				attachments = append(attachments, pkgmail.Attachment{FilePath: abs, Filename: a.OriginalName})
			}
		}
	} else {
		subject = "【测试邮件】会议室运营平台"
		body = `<div style="font-family:sans-serif;padding:24px">
			<h2>邮件配置测试成功 ✓</h2>
			<p>你好 ` + u.Name + `，</p>
			<p>这是一封来自会议室运营平台的测试邮件，表示 SMTP 配置工作正常。</p>
			<p style="color:#888;font-size:12px;margin-top:24px">发送时间: ` + time.Now().Format("2006-01-02 15:04:05") + `</p>
		</div>`
	}

	sender := pkgmail.NewSender(l.svcCtx.DB, l.svcCtx.Config)
	if err := sender.Send([]string{u.Email}, subject, body, inlineImages, attachments); err != nil {
		return nil, errors.New("发送失败: " + err.Error())
	}
	return &types.BaseResp{Message: "已发送到 " + u.Email}, nil
}
