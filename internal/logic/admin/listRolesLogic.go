package admin

import (
	"context"
	"encoding/json"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListRolesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListRolesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListRolesLogic {
	return &ListRolesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListRolesLogic) ListRoles() (resp *types.RoleListResp, err error) {
	var roles []model.Role
	if err = l.svcCtx.DB.Order("id").Find(&roles).Error; err != nil {
		return nil, err
	}
	resp = &types.RoleListResp{}
	for _, r := range roles {
		item := types.RoleItem{Id: r.Id, Name: r.Name, Label: r.Label}
		_ = json.Unmarshal([]byte(r.Menus), &item.Menus)
		_ = json.Unmarshal([]byte(r.Apis), &item.Apis)
		resp.List = append(resp.List, item)
	}
	return resp, nil
}
