package email

import (
	"context"
	"strings"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListEmailGroupsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListEmailGroupsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListEmailGroupsLogic {
	return &ListEmailGroupsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListEmailGroupsLogic) ListEmailGroups(req *types.EmailGroupListReq) (resp *types.EmailGroupListResp, err error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	where := ""
	args := []interface{}{}
	keyword := strings.TrimSpace(req.Keyword)
	if keyword != "" {
		where = " WHERE g.name LIKE ? OR g.scene LIKE ? "
		like := "%" + keyword + "%"
		args = append(args, like, like)
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM email_groups g" + where
	if err = l.svcCtx.DB.Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, err
	}

	var rows []struct {
		Id          int64
		Name        string
		HotelId     int64
		HotelName   string
		Scene       string
		MemberCount int
	}
	listArgs := append([]interface{}{}, args...)
	listArgs = append(listArgs, pageSize, (page-1)*pageSize)
	listSQL := `
		SELECT
			g.id         AS id,
			g.name       AS name,
			g.hotel_id   AS hotel_id,
			IFNULL(h.name,'') AS hotel_name,
			IFNULL(g.scene,'') AS scene,
			(SELECT COUNT(*) FROM email_group_members m WHERE m.group_id = g.id) AS member_count
		FROM email_groups g
		LEFT JOIN hotels h ON h.id = g.hotel_id
	` + where + `
		ORDER BY g.id DESC
		LIMIT ? OFFSET ?
	`
	if err = l.svcCtx.DB.Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	resp = &types.EmailGroupListResp{List: []types.EmailGroupItem{}, Total: total}
	for _, r := range rows {
		resp.List = append(resp.List, types.EmailGroupItem{
			Id:          r.Id,
			Name:        r.Name,
			HotelId:     r.HotelId,
			HotelName:   r.HotelName,
			Scene:       r.Scene,
			MemberCount: r.MemberCount,
		})
	}
	return resp, nil
}
