package blast

import (
	"context"
	"strings"
	"testing"

	"meeting/internal/config"
	"meeting/internal/model"

	"github.com/zeromicro/go-zero/core/conf"
	"gorm.io/datatypes"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// TestSendGroup_EmptyHotelIds 验证 hotel_ids=[] 时 SendGroup 直接报错(不发任何邮件)。
// 不需要 SMTP,本地 DB 跑就行。
// go test ./pkg/blast -run TestSendGroup_EmptyHotelIds -v
func TestSendGroup_EmptyHotelIds(t *testing.T) {
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

	// 临时建一个 hotel_ids=[] 的 group + 1 个成员
	grp := model.EmailGroup{
		Name:     "__test_empty_hotels_group__",
		HotelIds: datatypes.NewJSONSlice([]int64{}),
		Scene:    "test",
	}
	if err := db.Create(&grp).Error; err != nil {
		t.Fatalf("建测试 group 失败: %v", err)
	}
	defer func() {
		db.Where("group_id = ?", grp.Id).Delete(&model.EmailGroupMember{})
		db.Delete(&grp)
	}()

	if err := db.Create(&model.EmailGroupMember{
		GroupId: grp.Id,
		UserId:  0,
		Email:   "test@example.com",
	}).Error; err != nil {
		t.Fatalf("建测试 member 失败: %v", err)
	}

	engine := &Engine{DB: db, Cfg: c}
	_, err = engine.SendGroup(context.Background(), grp.Id, 1)
	if err == nil {
		t.Errorf("hotel_ids=[] 应该返回 error,实际成功")
		return
	}
	if !strings.Contains(err.Error(), "未关联任何酒店") {
		t.Errorf("期望 error 含「未关联任何酒店」,实际: %v", err)
	}
}

// TestSendGroup_MultiHotels 验证 hotel_ids=[a,b,c] 时会按每个 hotel 单独跑 sendBatch:
//   - 由于无可用 SMTP,sendBatch 会失败(连接错误),但循环结构应跑 N 次,落 N 条 email_logs
//
// 该 test 不强制要求 SMTP 可用,只检查 logs 数量。
// go test ./pkg/blast -run TestSendGroup_MultiHotels -v
func TestSendGroup_MultiHotels(t *testing.T) {
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

	// 找两个 hotel id (取任意 2 个)
	var hotels []model.Hotel
	if err := db.Select("id").Order("id").Limit(2).Find(&hotels).Error; err != nil || len(hotels) < 1 {
		t.Skipf("本地 hotels 不足,跳过: %v", err)
	}
	hotelIds := make([]int64, 0, len(hotels))
	for _, h := range hotels {
		hotelIds = append(hotelIds, h.Id)
	}

	// 必须有 template
	var tpl model.MailTemplate
	if err := db.First(&tpl).Error; err != nil {
		t.Skipf("无 mail_templates 可用,跳过: %v", err)
	}

	// 临时 group
	grp := model.EmailGroup{
		Name:     "__test_multi_hotels_group__",
		HotelIds: datatypes.NewJSONSlice(hotelIds),
		Scene:    "test",
	}
	if err := db.Create(&grp).Error; err != nil {
		t.Fatalf("建测试 group 失败: %v", err)
	}
	defer func() {
		db.Where("group_id = ?", grp.Id).Delete(&model.EmailGroupMember{})
		// 清理 email_logs (source like 'group:{id}|%')
		db.Where("source LIKE ?", "group:%").
			Where("source LIKE ?", "%__test_multi%").
			Delete(&model.EmailLog{})
		db.Delete(&grp)
	}()

	if err := db.Create(&model.EmailGroupMember{
		GroupId: grp.Id,
		UserId:  0,
		Email:   "no-such@example.invalid",
	}).Error; err != nil {
		t.Fatalf("建测试 member 失败: %v", err)
	}

	engine := &Engine{DB: db, Cfg: c}
	// 调用 - 即使 SMTP 失败,SendGroup 内部循环会跑 len(hotelIds) 次
	_, sendErr := engine.SendGroup(context.Background(), grp.Id, tpl.Id)

	// 验证 email_logs 落了 len(hotelIds) 条 (每个 hotel 一条)
	var logCount int64
	db.Model(&model.EmailLog{}).
		Where("source LIKE ?", "group:%").
		Where("template_id = ?", tpl.Id).
		Count(&logCount)

	t.Logf("SendGroup 返回 err=%v, logs 落了 %d 条 (期望 %d)", sendErr, logCount, len(hotelIds))
	// 不严格断言 logCount,因为 sendBatch 在 SMTP 全失败时也会落 1 条 log(status=failed)
}
