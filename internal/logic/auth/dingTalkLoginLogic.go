package auth

import (
	"context"
	"errors"
	"fmt"
	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"
	"meeting/pkg/dingtalk"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

type DingTalkLoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDingTalkLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DingTalkLoginLogic {
	return &DingTalkLoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DingTalkLoginLogic) DingTalkLogin(req *types.DingTalkLoginReq) (resp *types.DingTalkLoginResp, err error) {
	cfg := l.svcCtx.Config.DingTalk

	// 用 code 换取钉钉用户信息
	dtClient := &dingtalk.Client{
		AppKey:    cfg.AppKey,
		AppSecret: cfg.AppSecret,
	}
	userInfo, err := dtClient.GetUserByCode(req.Code)
	if err != nil {
		return nil, fmt.Errorf("钉钉免登失败: %w", err)
	}

	// 查或创建用户
	var user model.User
	result := l.svcCtx.DB.Where("dingtalk_union_id = ?", userInfo.UnionId).First(&user)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// 首次登录，自动注册并激活（管理员可在后台手动停用）
		user = model.User{
			DingTalkUnionId: userInfo.UnionId,
			Name:            userInfo.Name,
			Email:           userInfo.Email,
			Status:          "active",
			IsAdmin:         false,
		}
		if err := l.svcCtx.DB.Create(&user).Error; err != nil {
			return nil, fmt.Errorf("创建用户失败: %w", err)
		}
		l.Logger.Infof("新用户注册: %s (%s)", user.Name, user.DingTalkUnionId)
	} else if result.Error != nil {
		return nil, result.Error
	} else {
		// 已有用户：只更新姓名。
		// ❗email 不能在这里覆盖 —— 钉钉 GetUserByCode 对没授权邮箱 scope 的投手返回空 email,
		// 投手每次打开 H5 免登都会触发,会把 admin 在后台手填的 email 冲掉(历史 bug)。
		// email 只由 admin 后台「用户管理」维护。
		l.svcCtx.DB.Model(&user).Updates(map[string]interface{}{
			"name": userInfo.Name,
		})
	}

	// 查询该用户有权限的酒店列表
	var hotelIds []int64
	l.svcCtx.DB.Raw("SELECT hotel_id FROM user_hotel_perms WHERE user_id = ?", user.Id).Scan(&hotelIds)

	// 生成 JWT
	expireHour := l.svcCtx.Config.JWT.ExpireHour
	claims := jwt.MapClaims{
		"userId":  user.Id,
		"isAdmin": user.IsAdmin,
		"exp":     time.Now().Add(time.Duration(expireHour) * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(l.svcCtx.Config.JWT.Secret))
	if err != nil {
		return nil, fmt.Errorf("生成 token 失败: %w", err)
	}

	return &types.DingTalkLoginResp{
		Token:    tokenStr,
		Name:     user.Name,
		Status:   user.Status,
		HotelIds: hotelIds,
	}, nil
}
