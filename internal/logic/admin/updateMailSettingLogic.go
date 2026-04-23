package admin

import (
	"context"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateMailSettingLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateMailSettingLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateMailSettingLogic {
	return &UpdateMailSettingLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *UpdateMailSettingLogic) UpdateMailSetting(req *types.UpdateMailSettingReq) (resp *types.BaseResp, err error) {
	updates := map[string]interface{}{
		"smtp_host": req.SmtpHost,
		"smtp_port": req.SmtpPort,
		"username":  req.Username,
		"from_name": req.FromName,
	}
	if req.Password != "" {
		updates["password"] = req.Password
	}

	var count int64
	l.svcCtx.DB.Model(&model.MailSetting{}).Count(&count)
	if count == 0 {
		m := model.MailSetting{
			SmtpHost: req.SmtpHost,
			SmtpPort: req.SmtpPort,
			Username: req.Username,
			Password: req.Password,
			FromName: req.FromName,
		}
		err = l.svcCtx.DB.Create(&m).Error
	} else {
		err = l.svcCtx.DB.Model(&model.MailSetting{}).Where("id = 1").Updates(updates).Error
	}
	if err != nil {
		return nil, err
	}
	return &types.BaseResp{Message: "ok"}, nil
}
