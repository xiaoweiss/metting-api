// Package blast 模板变量装配 —— 每个收件人按"自己的主酒店 + 当日数据"独立渲染
package blast

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"meeting/internal/config"
	"meeting/internal/model"
	"meeting/pkg/mail"
)

// SkipReason 渲染阶段表达"跳过该 recipient"的原因
const (
	SkipReasonSnapshotMissing = "snapshot_missing" // 兼容旧调用方,等价于 image_missing
	SkipReasonImageMissing    = "image_missing"
	SkipReasonPDFMissing      = "pdf_missing"
)

// recipientVars 按 email 装配模板变量。所有动态字段都是字符串，没数据时填 "—"。
//
// hotelOverride > 0 时用它作为对标酒店（邮件组发送场景，按 group.hotel_id 覆盖）；
// 否则按收件人 users.primary_hotel_id 取；都没有的话 fallback 到关联酒店里第一家有 venue 的。
//
// 返回值:
//   - vars: 模板变量,包括 {{.DashboardImage}} / {{.DashboardPDF}}
//   - inlineImages: 命中 PNG snapshot 时本邮件需要 Embed 的图片列表
//   - attachments: 命中 PDF snapshot 时本邮件需要 Attach 的附件列表
//   - pngFound / pdfFound: 当日是否找到对应格式的 snapshot(供调用方决定是否 skip)
//   - hotelId: 解析出来的对标酒店 id(可能是 0)
//
// skip 由调用方根据模板内容决定:模板用了 {{.DashboardImage}} 但 !pngFound → skip
//                              模板用了 {{.DashboardPDF}} 但 !pdfFound → skip
//
// 模板里能用的变量:
//   - {{.Date}} / {{.Time}} / {{.UserName}} / {{.HotelName}} / {{.OccupancyRate}}
//   - {{.AM}} / {{.PM}} / {{.CompRate}} / {{.MarketRate}}
//   - {{.HotelRate}} / {{.MorningRate}} / {{.AfternoonRate}} / {{.CompetitorRate}}: 兼容旧名
//   - {{.DashboardImage}}: 本月日历 PNG 的 cid 引用,正文里 <img src="{{.DashboardImage}}">
//   - {{.DashboardPDF}}: 占位空串;模板里出现这个变量 → 系统自动当附件附上当日 PDF
func recipientVars(
	db *gorm.DB,
	cfg config.Config,
	email string,
	when time.Time,
	hotelOverride int64,
) (
	vars map[string]interface{},
	inlineImages []mail.InlineImage,
	attachments []mail.Attachment,
	pngFound bool,
	pdfFound bool,
	hotelId int64,
) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	dateStr := when.In(loc).Format("2006-01-02")

	vars = map[string]interface{}{
		"Date":           dateStr,
		"Time":           when.In(loc).Format("15:04"),
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
		"DashboardImage": "",
		"DashboardPDF":   "", // 占位空串;出现在模板里就是触发器
	}

	// 1) email → user(拿 UserName + primary_hotel_id)
	var user model.User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil || user.Id == 0 {
		if hotelOverride > 0 {
			fillHotelVars(db, vars, hotelOverride, dateStr)
			hotelId = hotelOverride
			loadDashboardSnapshots(db, cfg, vars, &inlineImages, &attachments, &pngFound, &pdfFound, hotelId, dateStr, loc)
		}
		return
	}
	if user.Name != "" {
		vars["UserName"] = user.Name
	}

	// 2) 决定对标酒店 id 的优先级
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
		return
	}

	fillHotelVars(db, vars, hotelId, dateStr)
	loadDashboardSnapshots(db, cfg, vars, &inlineImages, &attachments, &pngFound, &pdfFound, hotelId, dateStr, loc)
	return
}

// loadDashboardSnapshots 同时查 PNG 和 PDF 两种 format 的当日 snapshot,各自填到 inlineImages / attachments。
func loadDashboardSnapshots(
	db *gorm.DB,
	cfg config.Config,
	vars map[string]interface{},
	inlineImages *[]mail.InlineImage,
	attachments *[]mail.Attachment,
	pngFound *bool,
	pdfFound *bool,
	hotelId int64,
	dateStr string,
	loc *time.Location,
) {
	snapDate, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		return
	}
	var snaps []model.DashboardSnapshot
	db.Where("hotel_id = ? AND snapshot_date = ? AND mode = ?", hotelId, snapDate, "occupancy").
		Where("format IN ?", []string{"png", "pdf"}).
		Find(&snaps)
	for _, s := range snaps {
		abs := filepath.Join(cfg.Mail.SnapshotDir, s.FilePath)
		base := filepath.Base(s.FilePath)
		switch s.Format {
		case "png":
			vars["DashboardImage"] = "cid:" + base
			*inlineImages = append(*inlineImages, mail.InlineImage{FilePath: abs})
			*pngFound = true
		case "pdf":
			*attachments = append(*attachments, mail.Attachment{FilePath: abs, Filename: base})
			*pdfFound = true
		}
	}
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
