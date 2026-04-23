package admin

import (
	"context"
	"fmt"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
	"golang.org/x/crypto/bcrypt"
)

type CreateAdminUserLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateAdminUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateAdminUserLogic {
	return &CreateAdminUserLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *CreateAdminUserLogic) CreateAdminUser(req *types.CreateAdminUserReq) (*types.BaseResp, error) {
	var count int64
	l.svcCtx.DB.Model(&model.User{}).Where("name = ?", req.Username).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("用户名已存在")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := model.User{
		DingTalkUnionId: fmt.Sprintf("admin_%s", req.Username),
		Name:            req.Username,
		Email:           req.Email,
		Status:          "active",
		IsAdmin:         true,
		AdminPassword:   string(hashed),
	}
	if req.RoleId > 0 {
		user.RoleId = &req.RoleId
	}

	if err := l.svcCtx.DB.Create(&user).Error; err != nil {
		return nil, err
	}
	return &types.BaseResp{Message: "ok"}, nil
}
