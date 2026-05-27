package email

import (
	"context"
	"errors"
	"strings"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"
	"meeting/pkg/audit"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// diffEmails 计算 added / removed,不返回完整列表(避免 audit_logs.before/after 过大)
func diffEmails(before, after []string) (added, removed []string) {
	bm := make(map[string]struct{}, len(before))
	for _, e := range before {
		bm[e] = struct{}{}
	}
	am := make(map[string]struct{}, len(after))
	for _, e := range after {
		am[e] = struct{}{}
	}
	for e := range am {
		if _, ok := bm[e]; !ok {
			added = append(added, e)
		}
	}
	for e := range bm {
		if _, ok := am[e]; !ok {
			removed = append(removed, e)
		}
	}
	return
}

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

	hotelIds := dedupInt64(req.HotelIds)
	if err := validateHotelIdsExist(l.svcCtx.DB, hotelIds); err != nil {
		return nil, err
	}

	// audit: 记 before(name, hotel_ids, scene, members)
	var beforeGroup model.EmailGroup
	l.svcCtx.DB.First(&beforeGroup, req.Id)
	var beforeEmails []string
	l.svcCtx.DB.Model(&model.EmailGroupMember{}).
		Where("group_id = ?", req.Id).Pluck("email", &beforeEmails)
	var beforeHotelIds []int64
	for _, h := range beforeGroup.HotelIds {
		beforeHotelIds = append(beforeHotelIds, h)
	}

	err = l.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.EmailGroup{}).
			Where("id = ?", req.Id).
			Updates(map[string]interface{}{
				"name":      name,
				"hotel_ids": datatypes.NewJSONSlice(hotelIds),
				"scene":     strings.TrimSpace(req.Scene),
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

	// audit: 写 2 条 — email_groups (字段变更) + email_group_members (added/removed diff)
	var afterEmails []string
	l.svcCtx.DB.Model(&model.EmailGroupMember{}).
		Where("group_id = ?", req.Id).Pluck("email", &afterEmails)
	audit.Log(l.ctx, l.svcCtx.DB, audit.ActionUpdate, audit.TargetEmailGroups,
		req.Id, name,
		map[string]interface{}{
			"name":      beforeGroup.Name,
			"hotel_ids": beforeHotelIds,
			"scene":     beforeGroup.Scene,
		},
		map[string]interface{}{
			"name":      name,
			"hotel_ids": hotelIds,
			"scene":     strings.TrimSpace(req.Scene),
		})
	added, removed := diffEmails(beforeEmails, afterEmails)
	if len(added) > 0 || len(removed) > 0 {
		// before = 移除的成员, after = 新增的成员; 不存完整列表避免 audit 撑爆
		audit.Log(l.ctx, l.svcCtx.DB, audit.ActionUpdate, audit.TargetEmailGroupMembers,
			req.Id, name,
			map[string]interface{}{"removed": removed},
			map[string]interface{}{"added": added})
	}
	return &types.BaseResp{Message: "ok"}, nil
}
