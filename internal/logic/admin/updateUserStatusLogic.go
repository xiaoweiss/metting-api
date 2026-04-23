package admin

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

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
	result := l.svcCtx.DB.Exec(
		"UPDATE users SET status = ? WHERE id = ?", req.Status, req.Id,
	)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return &types.BaseResp{Code: 404, Message: "用户不存在"}, nil
	}
	return &types.BaseResp{Code: 0, Message: "ok"}, nil
}
