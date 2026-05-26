package email

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/datatypes"
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

	hotelIds := dedupInt64(req.HotelIds)
	if err := validateHotelIdsExist(l.svcCtx.DB, hotelIds); err != nil {
		return nil, err
	}

	err = l.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		g := model.EmailGroup{
			Name:     name,
			HotelIds: datatypes.NewJSONSlice(hotelIds),
			Scene:    strings.TrimSpace(req.Scene),
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

// dedupInt64 去重保序
func dedupInt64(ids []int64) []int64 {
	if len(ids) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// validateHotelIdsExist 检查所有 hotelId 都存在,允许空数组
func validateHotelIdsExist(db *gorm.DB, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	var count int64
	if err := db.Model(&model.Hotel{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(ids) {
		return fmt.Errorf("有 %d 个酒店不存在(已传 %d 个)", len(ids)-int(count), len(ids))
	}
	return nil
}
