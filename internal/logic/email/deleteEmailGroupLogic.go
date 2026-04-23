package email

import (
	"context"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

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
	return &types.BaseResp{Message: "ok"}, nil
}
