package admin

import (
	"context"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"
	"meeting/pkg/audit"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateUserStatusLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateUserStatusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserStatusLogic {
	return &UpdateUserStatusLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateUserStatusLogic) UpdateUserStatus(req *types.UpdateUserStatusReq) (resp *types.BaseResp, err error) {
	if req.Status != "active" && req.Status != "disabled" {
		return &types.BaseResp{Code: 400, Message: "status 只能为 active 或 disabled"}, nil
	}
	var before model.User
	l.svcCtx.DB.Select("id, name, status").First(&before, req.Id)
	result := l.svcCtx.DB.Exec(
		"UPDATE users SET status = ? WHERE id = ?", req.Status, req.Id,
	)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return &types.BaseResp{Code: 404, Message: "用户不存在"}, nil
	}
	audit.Log(l.ctx, l.svcCtx.DB, audit.ActionUpdate, audit.TargetUsers,
		before.Id, before.Name,
		map[string]string{"status": before.Status},
		map[string]string{"status": req.Status})
	return &types.BaseResp{Code: 0, Message: "ok"}, nil
}
