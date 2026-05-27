package admin

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"
	"meeting/pkg/audit"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateUserEmailLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateUserEmailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserEmailLogic {
	return &UpdateUserEmailLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func (l *UpdateUserEmailLogic) UpdateUserEmail(req *types.UpdateUserEmailReq) (resp *types.BaseResp, err error) {
	email := strings.TrimSpace(req.Email)
	if email != "" && !emailRe.MatchString(email) {
		return nil, errors.New("邮箱格式不正确")
	}
	var before model.User
	l.svcCtx.DB.Select("id, name, email").First(&before, req.Id)
	if err := l.svcCtx.DB.Table("users").Where("id = ?", req.Id).Update("email", email).Error; err != nil {
		return nil, err
	}
	audit.Log(l.ctx, l.svcCtx.DB, audit.ActionUpdate, audit.TargetUsers,
		before.Id, before.Name,
		map[string]string{"email": before.Email},
		map[string]string{"email": email})
	return &types.BaseResp{Message: "ok"}, nil
}
