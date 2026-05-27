package email

import (
	"context"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"
	"meeting/pkg/audit"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

type DeleteEmailGroupLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteEmailGroupLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteEmailGroupLogic {
	return &DeleteEmailGroupLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteEmailGroupLogic) DeleteEmailGroup(req *types.EmailGroupIdReq) (resp *types.BaseResp, err error) {
	// audit before
	var before model.EmailGroup
	l.svcCtx.DB.First(&before, req.Id)
	var beforeEmails []string
	l.svcCtx.DB.Model(&model.EmailGroupMember{}).
		Where("group_id = ?", req.Id).Pluck("email", &beforeEmails)

	err = l.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", req.Id).Delete(&model.EmailGroupMember{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", req.Id).Delete(&model.EmailGroup{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var beforeHotelIds []int64
	for _, h := range before.HotelIds {
		beforeHotelIds = append(beforeHotelIds, h)
	}
	audit.Log(l.ctx, l.svcCtx.DB, audit.ActionDelete, audit.TargetEmailGroups,
		req.Id, before.Name,
		map[string]interface{}{
			"name":      before.Name,
			"hotel_ids": beforeHotelIds,
			"scene":     before.Scene,
			"members":   beforeEmails,
		}, nil)
	return &types.BaseResp{Message: "ok"}, nil
}
