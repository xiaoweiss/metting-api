package admin

import (
	"context"
	"encoding/json"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateRoleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateRoleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateRoleLogic {
	return &UpdateRoleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateRoleLogic) UpdateRole(req *types.UpdateRoleReq) (resp *types.BaseResp, err error) {
	menusJSON, _ := json.Marshal(req.Menus)
	apisJSON, _ := json.Marshal(req.Apis)

	err = l.svcCtx.DB.Table("roles").Where("id = ?", req.Id).Updates(map[string]interface{}{
		"name":  req.Name,
		"label": req.Label,
		"menus": string(menusJSON),
		"apis":  string(apisJSON),
	}).Error
	if err != nil {
		return nil, err
	}
	return &types.BaseResp{Message: "ok"}, nil
}
