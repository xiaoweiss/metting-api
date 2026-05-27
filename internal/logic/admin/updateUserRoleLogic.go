package admin

import (
	"context"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"
	"meeting/pkg/audit"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateUserRoleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateUserRoleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserRoleLogic {
	return &UpdateUserRoleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateUserRoleLogic) UpdateUserRole(req *types.UpdateUserRoleReq) (resp *types.BaseResp, err error) {
	var roleId *int64
	if req.RoleId > 0 {
		roleId = &req.RoleId
	}
	var before model.User
	l.svcCtx.DB.Select("id, name, role_id").First(&before, req.Id)
	err = l.svcCtx.DB.Table("users").Where("id = ?", req.Id).Update("role_id", roleId).Error
	if err != nil {
		return nil, err
	}
	audit.Log(l.ctx, l.svcCtx.DB, audit.ActionUpdate, audit.TargetUsers,
		before.Id, before.Name,
		map[string]interface{}{"role_id": before.RoleId},
		map[string]interface{}{"role_id": roleId})
	return &types.BaseResp{Message: "ok"}, nil
}
