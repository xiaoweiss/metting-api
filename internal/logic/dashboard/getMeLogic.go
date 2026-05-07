package dashboard

import (
	"context"
	"encoding/json"
	"strconv"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetMeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMeLogic {
	return &GetMeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetMeLogic) GetMe(r interface{ Header() interface{ Get(string) string } }) (resp *types.UserInfoResp, err error) {
	return nil, nil // 由 handler 层直接读 header
}

// GetMeByUserId 供 handler 调用
func GetMeByUserId(ctx context.Context, svcCtx *svc.ServiceContext, userIdStr string) (*types.UserInfoResp, error) {
	userId, err := strconv.ParseInt(userIdStr, 10, 64)
	if err != nil {
		return nil, err
	}

	var user struct {
		Id       int64
		Name     string
		Email    string
		Status   string
		IsAdmin  bool
		RoleId   *int64
		RoleName *string
		Menus    *string
	}
	if err := svcCtx.DB.Raw(`
		SELECT u.id, u.name, u.email, u.status, u.is_admin, u.role_id,
		       r.name AS role_name, r.menus
		FROM users u
		LEFT JOIN roles r ON r.id = u.role_id
		WHERE u.id = ?
	`, userId).Scan(&user).Error; err != nil {
		return nil, err
	}

	var hotelIds []int64
	svcCtx.DB.Raw("SELECT hotel_id FROM user_hotel_perms WHERE user_id = ?", userId).Scan(&hotelIds)

	var hotels []types.UserHotelItem
	if len(hotelIds) > 0 {
		svcCtx.DB.Raw("SELECT id, name, COALESCE(city,'') AS city FROM hotels WHERE id IN ?", hotelIds).Scan(&hotels)
	}

	resp := &types.UserInfoResp{
		Id:       user.Id,
		Name:     user.Name,
		Email:    user.Email,
		Status:   user.Status,
		HotelIds: hotelIds,
		Hotels:   hotels,
		IsAdmin:  user.IsAdmin,
	}

	if user.RoleId != nil {
		resp.RoleId = *user.RoleId
	}
	if user.RoleName != nil {
		resp.RoleName = *user.RoleName
	}
	if user.Menus != nil {
		_ = json.Unmarshal([]byte(*user.Menus), &resp.Menus)
	}

	return resp, nil
}
