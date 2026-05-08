// Package blast 模板变量装配 —— 每个收件人按"自己的主酒店 + 当日数据"独立渲染
package blast

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"meeting/internal/model"
)

// recipientVars 按 email 装配模板变量。所有动态字段都是字符串，没数据时填 "—"。
//
// hotelOverride > 0 时用它作为对标酒店（邮件组发送场景，按 group.hotel_id 覆盖）；
// 否则按收件人 users.primary_hotel_id 取；都没有的话 fallback 到关联酒店里第一家有 venue 的。
//
// 模板里能用的变量：
//   - {{.Date}}            报告日期 YYYY-MM-DD
//   - {{.Time}}            发送时间 HH:MM
//   - {{.UserName}}        收件人姓名
//   - {{.HotelName}}       对标酒店名
//   - {{.OccupancyRate}}   该酒店当日综合出租率
//   - {{.AM}}              上午出租率
//   - {{.PM}}              下午出租率
//   - {{.CompRate}}        竞对群当日均值
//   - {{.MarketRate}}      商圈当日均值
//   - {{.HotelRate}}       同 OccupancyRate（保留旧名字向后兼容）
//   - {{.MorningRate}}     同 AM（向后兼容）
//   - {{.AfternoonRate}}   同 PM
//   - {{.CompetitorRate}}  同 CompRate
func recipientVars(db *gorm.DB, email string, when time.Time, hotelOverride int64) map[string]interface{} {
	dateStr := when.Format("2006-01-02")
	vars := map[string]interface{}{
		"Date":           dateStr,
		"Time":           when.Format("15:04"),
		"UserName":       defaultUserName(email),
		"HotelName":      "—",
		"OccupancyRate":  "—",
		"AM":             "—",
		"PM":             "—",
		"CompRate":       "—",
		"MarketRate":     "—",
		"HotelRate":      "—",
		"MorningRate":    "—",
		"AfternoonRate":  "—",
		"CompetitorRate": "—",
	}

	// 1) email → user（拿 UserName + primary_hotel_id）
	var user model.User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil || user.Id == 0 {
		// 不是系统用户：如果 hotelOverride 给了，仍然按它渲染酒店字段
		if hotelOverride > 0 {
			fillHotelVars(db, vars, hotelOverride, dateStr)
		}
		return vars
	}
	if user.Name != "" {
		vars["UserName"] = user.Name
	}

	// 2) 决定对标酒店 id 的优先级：
	//    a. hotelOverride（邮件组发送时 group.hotel_id 覆盖）
	//    b. user.primary_hotel_id（"所属酒店"）
	//    c. user_hotel_perms 里第一家有 venue 的（兜底）
	//    d. user_hotel_perms 里 hotel_id 最小的（再兜底，至少能渲染 HotelName）
	var hotelId int64
	switch {
	case hotelOverride > 0:
		hotelId = hotelOverride
	case user.PrimaryHotelId != nil && *user.PrimaryHotelId > 0:
		hotelId = *user.PrimaryHotelId
	default:
		db.Raw(`
			SELECT p.hotel_id
			FROM user_hotel_perms p
			WHERE p.user_id = ?
			  AND EXISTS (SELECT 1 FROM venues v WHERE v.hotel_id = p.hotel_id)
			ORDER BY p.hotel_id LIMIT 1
		`, user.Id).Scan(&hotelId)
		if hotelId == 0 {
			db.Raw("SELECT hotel_id FROM user_hotel_perms WHERE user_id = ? ORDER BY hotel_id LIMIT 1", user.Id).Scan(&hotelId)
		}
	}
	if hotelId == 0 {
		return vars
	}
	fillHotelVars(db, vars, hotelId, dateStr)
	return vars
}

// fillHotelVars 给定 hotelId + 日期，把酒店相关变量填到 vars。
// 拆出来是因为 hotelOverride 场景下哪怕没 user 也要走这个填充。
func fillHotelVars(db *gorm.DB, vars map[string]interface{}, hotelId int64, dateStr string) {
	var hotel model.Hotel
	if err := db.First(&hotel, hotelId).Error; err == nil {
		vars["HotelName"] = hotel.Name
	}

	// 3) 当日酒店出租率（venue_type 不限）
	var hotelTotalVenues int
	db.Raw("SELECT COUNT(*) FROM venues WHERE hotel_id = ?", hotelId).Scan(&hotelTotalVenues)
	if hotelTotalVenues == 0 {
		return
	}

	type periodSum struct {
		Period string
		Booked int
	}
	var hotelRows []periodSum
	db.Raw(`
		SELECT period, SUM(is_booked) AS booked
		FROM meeting_records
		WHERE hotel_id = ? AND DATE(record_date) = ?
		GROUP BY period
	`, hotelId, dateStr).Scan(&hotelRows)

	var amB, pmB int
	for _, r := range hotelRows {
		switch r.Period {
		case "AM":
			amB = r.Booked
		case "PM":
			pmB = r.Booked
		}
	}
	am := pct(amB, hotelTotalVenues)
	pm := pct(pmB, hotelTotalVenues)
	overall := pct(amB+pmB, hotelTotalVenues*2)
	vars["AM"] = am
	vars["PM"] = pm
	vars["OccupancyRate"] = overall
	vars["MorningRate"] = am
	vars["AfternoonRate"] = pm
	vars["HotelRate"] = overall

	// 4) 竞对当日均值
	var compTotal int
	db.Raw(`
		SELECT COUNT(*) FROM venues v
		JOIN competitor_group_hotels cgh ON cgh.hotel_id = v.hotel_id
		JOIN competitor_groups cg ON cg.id = cgh.group_id AND cg.base_hotel_id = ?
	`, hotelId).Scan(&compTotal)
	if compTotal > 0 {
		var compBooked int
		db.Raw(`
			SELECT IFNULL(SUM(mr.is_booked), 0)
			FROM meeting_records mr
			JOIN competitor_group_hotels cgh ON cgh.hotel_id = mr.hotel_id
			JOIN competitor_groups cg ON cg.id = cgh.group_id AND cg.base_hotel_id = ?
			WHERE DATE(mr.record_date) = ?
		`, hotelId, dateStr).Scan(&compBooked)
		comp := pct(compBooked, compTotal*2)
		vars["CompRate"] = comp
		vars["CompetitorRate"] = comp
	}

	// 5) 商圈当日均值
	var marketTotal int
	db.Raw(`
		SELECT COUNT(*) FROM venues v
		JOIN hotels h ON h.id = v.hotel_id
		WHERE h.market_area_id = (SELECT market_area_id FROM hotels WHERE id = ?)
	`, hotelId).Scan(&marketTotal)
	if marketTotal > 0 {
		var marketBooked int
		db.Raw(`
			SELECT IFNULL(SUM(mr.is_booked), 0)
			FROM meeting_records mr
			JOIN hotels h ON h.id = mr.hotel_id
			WHERE h.market_area_id = (SELECT market_area_id FROM hotels WHERE id = ?)
			  AND DATE(mr.record_date) = ?
		`, hotelId, dateStr).Scan(&marketBooked)
		vars["MarketRate"] = pct(marketBooked, marketTotal*2)
	}
}

// defaultUserName 在 users 表里查不到时，用邮箱 @ 前面的部分当姓名占位
func defaultUserName(email string) string {
	if i := strings.Index(email, "@"); i > 0 {
		return email[:i]
	}
	return email
}

func pct(numer, denom int) string {
	if denom <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d%%", numer*100/denom)
}
