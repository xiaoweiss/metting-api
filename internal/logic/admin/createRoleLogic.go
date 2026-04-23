package admin

import (
	"context"
	"encoding/json"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateRoleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateRoleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateRoleLogic {
	return &CreateRoleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateRoleLogic) CreateRole(req *types.CreateRoleReq) (resp *types.BaseResp, err error) {
	menusJSON, _ := json.Marshal(req.Menus)
	apisJSON, _ := json.Marshal(req.Apis)

	role := model.Role{
		Name:  req.Name,
		Label: req.Label,
		Menus: string(menusJSON),
		Apis:  string(apisJSON),
	}
	if err = l.svcCtx.DB.Create(&role).Error; err != nil {
		return nil, err
	}
	return &types.BaseResp{Message: "ok"}, nil
}
