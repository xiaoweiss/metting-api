package sync

import (
	"context"
	"fmt"

	"meeting/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// syncUserPerms 从汇总表同步酒店对接人员 → users + user_hotel_perms
func (e *Engine) syncUserPerms(ctx context.Context) error {
	sheetId := e.cfg.DingTalk.Sheet.Sheets.Summary
	rows, err := e.sheet.WithWorksheet(sheetId).GetAllRows()
	if err != nil {
		return fmt.Errorf("读取汇总表失败: %w", err)
	}

	// 酒店名 → id
	hotelNameToId := make(map[string]int64)
	var hotels []model.Hotel
	e.db.Find(&hotels)
	for _, h := range hotels {
		hotelNameToId[h.Name] = h.Id
	}

	permCount := 0
	for _, row := range rows {
		hotelName := textField(row, "酒店名称 Hotel Name")
		users := userFields(row, "酒店对接人员")

		if hotelName == "" || len(users) == 0 {
			continue
		}

		hotelId, ok := hotelNameToId[hotelName]
		if !ok {
			logx.Infof("[syncUserPerms] 酒店 '%s' 未找到，跳过", hotelName)
			continue
		}

		for _, u := range users {
			// upsert 用户
			var user model.User
			result := e.db.Where("dingtalk_union_id = ?", u.UnionId).First(&user)
			if result.Error != nil {
				// 新用户，直接 active（因为是对接人员）
				user = model.User{
					DingTalkUnionId: u.UnionId,
					Name:            u.Name,
					Status:          "active",
					IsAdmin:         false,
				}
				e.db.Create(&user)
			} else {
				e.db.Model(&user).Updates(map[string]interface{}{
					"name":   u.Name,
					"status": "active",
				})
			}

			// upsert 权限
			var perm model.UserHotelPerm
			e.db.Where("user_id = ? AND hotel_id = ?", user.Id, hotelId).
				FirstOrCreate(&perm, model.UserHotelPerm{
					UserId:  user.Id,
					HotelId: hotelId,
				})
			permCount++
		}
	}

	e.logSync("user_perms", "success", permCount, fmt.Sprintf("同步 %d 条权限", permCount))
	logx.Infof("[syncUserPerms] 完成，%d 条权限", permCount)
	return nil
}
