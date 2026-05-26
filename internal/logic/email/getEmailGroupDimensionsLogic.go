package email

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetEmailGroupDimensionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetEmailGroupDimensionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetEmailGroupDimensionsLogic {
	return &GetEmailGroupDimensionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetEmailGroupDimensions 给前端「按维度批量加成员」用,
// 返回 hotels 表 distinct 的 hotel_group / hotel_type 取值列表(过滤空)。
func (l *GetEmailGroupDimensionsLogic) GetEmailGroupDimensions() (resp *types.EmailGroupDimensionsResp, err error) {
	resp = &types.EmailGroupDimensionsResp{
		Groups: []string{},
		Types:  []string{},
	}

	if err := l.svcCtx.DB.Raw(`
		SELECT DISTINCT hotel_group FROM hotels
		WHERE hotel_group IS NOT NULL AND hotel_group <> ''
		ORDER BY hotel_group
	`).Scan(&resp.Groups).Error; err != nil {
		return nil, err
	}

	if err := l.svcCtx.DB.Raw(`
		SELECT DISTINCT hotel_type FROM hotels
		WHERE hotel_type IS NOT NULL AND hotel_type <> ''
		ORDER BY hotel_type
	`).Scan(&resp.Types).Error; err != nil {
		return nil, err
	}

	if resp.Groups == nil {
		resp.Groups = []string{}
	}
	if resp.Types == nil {
		resp.Types = []string{}
	}
	return resp, nil
}
