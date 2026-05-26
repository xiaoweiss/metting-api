package email

import (
	"context"
	"errors"
	"strings"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PreviewEmailGroupMembersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPreviewEmailGroupMembersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PreviewEmailGroupMembersLogic {
	return &PreviewEmailGroupMembersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// PreviewEmailGroupMembers 按 dimension(group/type) + value 拉满足条件的 active 用户列表,
// 用 users.primary_hotel_id JOIN hotels 过滤。
// 白名单 dimension ∈ {"group","type"},非法返回 400(由 handler 转 errors.New)。
func (l *PreviewEmailGroupMembersLogic) PreviewEmailGroupMembers(req *types.PreviewEmailGroupMembersReq) (resp *types.PreviewEmailGroupMembersResp, err error) {
	dim := strings.TrimSpace(req.Dimension)
	val := strings.TrimSpace(req.Value)

	var col string
	switch dim {
	case "group":
		col = "h.hotel_group"
	case "type":
		col = "h.hotel_type"
	default:
		return nil, errors.New("dimension 非法,仅支持 group / type")
	}
	if val == "" {
		return nil, errors.New("value 不能为空")
	}

	resp = &types.PreviewEmailGroupMembersResp{
		List: []types.PreviewEmailGroupMemberItem{},
	}

	sql := `
		SELECT
			u.id            AS id,
			u.name          AS name,
			u.email         AS email,
			u.primary_hotel_id AS hotel_id,
			h.name          AS hotel_name,
			h.hotel_group   AS hotel_group,
			h.hotel_type    AS hotel_type
		FROM users u
		INNER JOIN hotels h ON h.id = u.primary_hotel_id
		WHERE u.email IS NOT NULL AND u.email <> ''
		  AND u.status = 'active'
		  AND ` + col + ` = ?
		ORDER BY h.hotel_group, h.name, u.name
	`
	if err := l.svcCtx.DB.Raw(sql, val).Scan(&resp.List).Error; err != nil {
		return nil, err
	}
	resp.Count = len(resp.List)
	return resp, nil
}
