package admin

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListAdminUsersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListAdminUsersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAdminUsersLogic {
	return &ListAdminUsersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListAdminUsersLogic) ListAdminUsers() (resp *types.AdminUserListResp, err error) {
	var users []struct {
		Id      int64
		Name    string
		Email   string
		Status  string
		IsAdmin bool
		RoleId  *int64
	}
	if err = l.svcCtx.DB.Raw(
		"SELECT id, name, email, status, is_admin, role_id FROM users ORDER BY created_at DESC",
	).Scan(&users).Error; err != nil {
		return nil, err
	}

	// 批量查询每个用户绑定的酒店 id
	var perms []struct {
		UserId  int64
		HotelId int64
	}
	l.svcCtx.DB.Raw("SELECT user_id, hotel_id FROM user_hotel_perms").Scan(&perms)
	permMap := map[int64][]int64{}
	for _, p := range perms {
		permMap[p.UserId] = append(permMap[p.UserId], p.HotelId)
	}

	resp = &types.AdminUserListResp{}
	for _, u := range users {
		item := types.AdminUserItem{
			Id:       u.Id,
			Name:     u.Name,
			Email:    u.Email,
			Status:   u.Status,
			HotelIds: permMap[u.Id],
		}
		if u.RoleId != nil {
			item.RoleId = *u.RoleId
		}
		resp.List = append(resp.List, item)
	}
	return resp, nil
}
