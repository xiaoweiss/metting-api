package admin

import (
	"context"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMailSettingLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetMailSettingLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMailSettingLogic {
	return &GetMailSettingLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *GetMailSettingLogic) GetMailSetting() (resp *types.MailSettingResp, err error) {
	var m model.MailSetting
	if err = l.svcCtx.DB.First(&m).Error; err != nil {
		return &types.MailSettingResp{
			SmtpHost: l.svcCtx.Config.Mail.Host,
			SmtpPort: l.svcCtx.Config.Mail.Port,
			Username: l.svcCtx.Config.Mail.Username,
			FromName: l.svcCtx.Config.Mail.FromName,
		}, nil
	}
	return &types.MailSettingResp{
		SmtpHost: m.SmtpHost,
		SmtpPort: m.SmtpPort,
		Username: m.Username,
		Password: maskPassword(m.Password),
		FromName: m.FromName,
	}, nil
}

func maskPassword(p string) string {
	if len(p) <= 4 {
		return "****"
	}
	return p[:2] + "****" + p[len(p)-2:]
}
