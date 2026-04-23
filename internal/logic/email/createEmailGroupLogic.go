package email

import (
	"context"
	"errors"
	"strings"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

type CreateEmailGroupLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateEmailGroupLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateEmailGroupLogic {
	return &CreateEmailGroupLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateEmailGroupLogic) CreateEmailGroup(req *types.CreateEmailGroupReq) (resp *types.BaseResp, err error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("名称不能为空")
	}

	err = l.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		g := model.EmailGroup{
			Name:    name,
			HotelId: req.HotelId,
			Scene:   strings.TrimSpace(req.Scene),
		}
		if err := tx.Create(&g).Error; err != nil {
			return err
		}
		for _, m := range req.Members {
			email := strings.TrimSpace(m.Email)
			if email == "" {
				// 兜底：若前端未带 email，则从 users 表查
				var u model.User
				if err := tx.Select("email").Where("id = ?", m.UserId).First(&u).Error; err == nil {
					email = u.Email
				}
			}
			if email == "" {
				continue // 跳过无邮箱成员
			}
			if err := tx.Create(&model.EmailGroupMember{
				GroupId: g.Id,
				UserId:  m.UserId,
				Email:   email,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &types.BaseResp{Message: "ok"}, nil
}
