package blast

import (
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/zeromicro/go-zero/core/conf"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"meeting/internal/config"
)

// 集成 smoke test: 需要本地 MySQL 跑 + 已应用 013 migration + 至少一行 dashboard_snapshots。
// go test ./pkg/blast -run TestRecipientVars_DashboardImage -v -tags=integration
func TestRecipientVars_DashboardImage(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}
	flag.Parse()

	var c config.Config
	if err := conf.Load("../../etc/meeting-api.yaml", &c); err != nil {
		t.Skipf("缺少配置文件: %v", err)
	}

	db, err := gorm.Open(mysql.Open(c.DB.DSN), &gorm.Config{})
	if err != nil {
		t.Skipf("DB 连接失败: %v", err)
	}

	// 用 1369030562@qq.com (本地 user id=1, primary_hotel_id=1) + 今日 (Asia/Shanghai)
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, loc)
	vars, inlineImages, attachments, pngFound, pdfFound, hotelId := recipientVars(db, c, "1369030562@qq.com", now, 0)

	fmt.Printf("vars[Date]=%v\nvars[HotelName]=%v\nDashboardImage=%v\npngFound=%v pdfFound=%v\nhotelId=%v\ninlineImages=%+v\nattachments=%+v\n",
		vars["Date"], vars["HotelName"], vars["DashboardImage"],
		pngFound, pdfFound, hotelId, inlineImages, attachments)

	if hotelId != 1 {
		t.Errorf("hotelId 应=1, 实际=%d", hotelId)
	}
	if !pngFound {
		t.Errorf("本地数据有 PNG snapshot, 期望 pngFound=true")
	}
	if vars["DashboardImage"] != "cid:dashboard-1-2026-05-09-occupancy.png" {
		t.Errorf("DashboardImage 期望 cid:..., 实际=%v", vars["DashboardImage"])
	}
	if len(inlineImages) != 1 {
		t.Errorf("inlineImages 应=1, 实际=%d", len(inlineImages))
	}
}

// 缺图场景: 用一个不存在的日期
func TestRecipientVars_SnapshotMissing(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}
	var c config.Config
	if err := conf.Load("../../etc/meeting-api.yaml", &c); err != nil {
		t.Skipf("缺少配置文件: %v", err)
	}
	db, err := gorm.Open(mysql.Open(c.DB.DSN), &gorm.Config{})
	if err != nil {
		t.Skipf("DB 连接失败: %v", err)
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	missingDay := time.Date(2099, 1, 1, 12, 0, 0, 0, loc)
	_, inlineImages, attachments, pngFound, pdfFound, hotelId := recipientVars(db, c, "1369030562@qq.com", missingDay, 0)

	if pngFound || pdfFound {
		t.Errorf("缺图应该 pngFound=false pdfFound=false, 实际 png=%v pdf=%v", pngFound, pdfFound)
	}
	if hotelId != 1 {
		t.Errorf("hotelId 应=1, 实际=%d", hotelId)
	}
	if len(inlineImages) != 0 || len(attachments) != 0 {
		t.Errorf("inlineImages/attachments 应空, 实际 inline=%d att=%d", len(inlineImages), len(attachments))
	}
}
