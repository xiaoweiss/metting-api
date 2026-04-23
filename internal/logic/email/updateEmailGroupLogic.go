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

type UpdateEmailGroupLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateEmailGroupLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateEmailGroupLogic {
	return &UpdateEmailGroupLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateEmailGroupLogic) UpdateEmailGroup(req *types.UpdateEmailGroupReq) (resp *types.BaseResp, err error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("名称不能为空")
	}

	err = l.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.EmailGroup{}).
			Where("id = ?", req.Id).
			Updates(map[string]interface{}{
				"name":     name,
				"hotel_id": req.HotelId,
				"scene":    strings.TrimSpace(req.Scene),
			}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", req.Id).Delete(&model.EmailGroupMember{}).Error; err != nil {
			return err
		}
		for _, m := range req.Members {
			email := strings.TrimSpace(m.Email)
			if email == "" {
				var u model.User
				if err := tx.Select("email").Where("id = ?", m.UserId).First(&u).Error; err == nil {
					email = u.Email
				}
			}
			if email == "" {
				continue
			}
			if err := tx.Create(&model.EmailGroupMember{
				GroupId: req.Id,
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
