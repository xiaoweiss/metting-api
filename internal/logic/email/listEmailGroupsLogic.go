package email

import (
	"context"
	"encoding/json"
	"strings"

	"meeting/internal/model"
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

	// 1. 查 group 基础信息(hotel_ids 保留 JSON 字符串后续解析,避免 GORM 的 JSON.Slice scan 兼容问题)
	var rows []struct {
		Id          int64
		Name        string
		HotelIdsRaw string `gorm:"column:hotel_ids"`
		Scene       string
		MemberCount int
	}
	listArgs := append([]interface{}{}, args...)
	listArgs = append(listArgs, pageSize, (page-1)*pageSize)
	listSQL := `
		SELECT
			g.id         AS id,
			g.name       AS name,
			g.hotel_ids  AS hotel_ids,
			IFNULL(g.scene,'') AS scene,
			(SELECT COUNT(*) FROM email_group_members m WHERE m.group_id = g.id) AS member_count
		FROM email_groups g
	` + where + `
		ORDER BY g.id DESC
		LIMIT ? OFFSET ?
	`
	if err = l.svcCtx.DB.Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	// 2. 收集所有出现过的 hotelId,一次性 IN 查 name(N+1 -> 1)
	allHotelIdSet := map[int64]struct{}{}
	parsedHotelIds := make([][]int64, len(rows))
	for i, r := range rows {
		var ids []int64
		if r.HotelIdsRaw != "" {
			_ = json.Unmarshal([]byte(r.HotelIdsRaw), &ids)
		}
		parsedHotelIds[i] = ids
		for _, id := range ids {
			allHotelIdSet[id] = struct{}{}
		}
	}
	hotelNameMap := map[int64]string{}
	if len(allHotelIdSet) > 0 {
		hotelIds := make([]int64, 0, len(allHotelIdSet))
		for id := range allHotelIdSet {
			hotelIds = append(hotelIds, id)
		}
		var hotels []model.Hotel
		l.svcCtx.DB.Select("id, name").Where("id IN ?", hotelIds).Find(&hotels)
		for _, h := range hotels {
			hotelNameMap[h.Id] = h.Name
		}
	}

	resp = &types.EmailGroupListResp{List: []types.EmailGroupItem{}, Total: total}
	for i, r := range rows {
		ids := parsedHotelIds[i]
		if ids == nil {
			ids = []int64{}
		}
		names := make([]string, 0, len(ids))
		for _, id := range ids {
			if n, ok := hotelNameMap[id]; ok && n != "" {
				names = append(names, n)
			} else {
				// 铁规 6: id 拉不到时兜底"酒店 #N",不裸露纯 id
				names = append(names, "酒店 #"+itoa(id))
			}
		}
		resp.List = append(resp.List, types.EmailGroupItem{
			Id:          r.Id,
			Name:        r.Name,
			HotelIds:    ids,
			HotelNames:  names,
			Scene:       r.Scene,
			MemberCount: r.MemberCount,
		})
	}
	return resp, nil
}

func itoa(n int64) string {
	// 简单 fmt.Sprintf 替代,避免额外 import
	s := ""
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		s = string('0'+byte(n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}
