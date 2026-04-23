package admin

import (
	"context"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type MailTemplateLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewMailTemplateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MailTemplateLogic {
	return &MailTemplateLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *MailTemplateLogic) List() (*types.MailTemplateListResp, error) {
	var tpls []model.MailTemplate
	if err := l.svcCtx.DB.Order("id").Find(&tpls).Error; err != nil {
		return nil, err
	}
	resp := &types.MailTemplateListResp{}
	for _, t := range tpls {
		resp.List = append(resp.List, types.MailTemplateItem{
			Id: t.Id, Name: t.Name, Subject: t.Subject, Body: t.Body, Description: t.Description,
		})
	}
	return resp, nil
}

func (l *MailTemplateLogic) Create(req *types.CreateMailTemplateReq) (*types.BaseResp, error) {
	t := model.MailTemplate{Name: req.Name, Subject: req.Subject, Body: req.Body, Description: req.Description}
	if err := l.svcCtx.DB.Create(&t).Error; err != nil {
		return nil, err
	}
	return &types.BaseResp{Message: "ok"}, nil
}

func (l *MailTemplateLogic) Update(req *types.UpdateMailTemplateReq) (*types.BaseResp, error) {
	err := l.svcCtx.DB.Model(&model.MailTemplate{}).Where("id = ?", req.Id).Updates(map[string]interface{}{
		"name": req.Name, "subject": req.Subject, "body": req.Body, "description": req.Description,
	}).Error
	if err != nil {
		return nil, err
	}
	return &types.BaseResp{Message: "ok"}, nil
}

func (l *MailTemplateLogic) Delete(id int64) (*types.BaseResp, error) {
	if err := l.svcCtx.DB.Delete(&model.MailTemplate{}, id).Error; err != nil {
		return nil, err
	}
	return &types.BaseResp{Message: "ok"}, nil
}
