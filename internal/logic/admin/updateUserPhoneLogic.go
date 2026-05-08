package admin

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateUserPhoneLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateUserPhoneLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserPhoneLogic {
	return &UpdateUserPhoneLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// 中国大陆手机号 11 位，1 开头第 2 位 3-9
var phoneRe = regexp.MustCompile(`^1[3-9]\d{9}$`)

func (l *UpdateUserPhoneLogic) UpdateUserPhone(req *types.UpdateUserPhoneReq) (resp *types.BaseResp, err error) {
	phone := strings.TrimSpace(req.Phone)
	if phone != "" && !phoneRe.MatchString(phone) {
		return nil, errors.New("手机号格式不正确（11 位，1 开头）")
	}
	if err := l.svcCtx.DB.Table("users").Where("id = ?", req.Id).Update("phone", phone).Error; err != nil {
		return nil, err
	}
	return &types.BaseResp{Message: "ok"}, nil
}
