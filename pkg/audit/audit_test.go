package audit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"meeting/internal/config"
	"meeting/internal/model"

	"github.com/zeromicro/go-zero/core/conf"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// connectDB 连本地 dev DB (ding), 跳过用 -short
func connectDB(t *testing.T) *gorm.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("跳过集成测试")
	}
	var c config.Config
	cfgPath := "../../etc/meeting-api.yaml"
	if _, err := os.Stat(cfgPath); err != nil {
		t.Skipf("缺配置文件 %s: %v", cfgPath, err)
	}
	if err := conf.Load(cfgPath, &c); err != nil {
		t.Skipf("配置加载失败: %v", err)
	}
	db, err := gorm.Open(mysql.Open(c.DB.DSN), &gorm.Config{})
	if err != nil {
		t.Skipf("DB 连接失败: %v", err)
	}
	// 校验表存在 (migration 017 已跑)
	if !db.Migrator().HasTable(&model.AuditLog{}) {
		t.Skipf("audit_logs 表不存在,先跑 migration 017")
	}
	return db
}

// cleanup 删测试残留 (按 target_name 前缀)
func cleanup(t *testing.T, db *gorm.DB, prefix string) {
	t.Helper()
	db.Where("target_name LIKE ?", prefix+"%").Delete(&model.AuditLog{})
}

func TestLog_Create(t *testing.T) {
	db := connectDB(t)
	prefix := "test_audit_create_"
	cleanup(t, db, prefix)
	defer cleanup(t, db, prefix)

	ctx := WithActor(context.Background(), 1, "alice", "192.168.0.1")
	target := prefix + time.Now().Format("150405")
	Log(ctx, db, ActionCreate, TargetUsers, 999, target, nil,
		map[string]string{"name": "test"})

	var rec model.AuditLog
	if err := db.Where("target_name = ?", target).First(&rec).Error; err != nil {
		t.Fatalf("audit log 未写入: %v", err)
	}
	if rec.UserId == nil || *rec.UserId != 1 {
		t.Errorf("UserId mismatch: %v", rec.UserId)
	}
	if rec.UserName != "alice" {
		t.Errorf("UserName mismatch: %s", rec.UserName)
	}
	if rec.Action != ActionCreate {
		t.Errorf("Action mismatch: %s", rec.Action)
	}
	if rec.TargetType != TargetUsers {
		t.Errorf("TargetType mismatch: %s", rec.TargetType)
	}
	if rec.Ip != "192.168.0.1" {
		t.Errorf("Ip mismatch: %s", rec.Ip)
	}
	if len(rec.BeforeValue) > 0 && string(rec.BeforeValue) != "null" {
		t.Errorf("create 时 before 应为空, 实际 %s", string(rec.BeforeValue))
	}
	if len(rec.AfterValue) == 0 {
		t.Errorf("after 不应为空")
	}
}

func TestLog_Update(t *testing.T) {
	db := connectDB(t)
	prefix := "test_audit_update_"
	cleanup(t, db, prefix)
	defer cleanup(t, db, prefix)

	ctx := WithActor(context.Background(), 2, "bob", "10.0.0.1")
	target := prefix + time.Now().Format("150405")
	Log(ctx, db, ActionUpdate, TargetMailTemplates, 7, target,
		map[string]string{"subject": "old"},
		map[string]string{"subject": "new"})

	var rec model.AuditLog
	if err := db.Where("target_name = ?", target).First(&rec).Error; err != nil {
		t.Fatalf("audit log 未写入: %v", err)
	}
	if rec.UserId == nil || *rec.UserId != 2 {
		t.Errorf("UserId mismatch")
	}
	var before, after map[string]string
	json.Unmarshal(rec.BeforeValue, &before)
	json.Unmarshal(rec.AfterValue, &after)
	if before["subject"] != "old" || after["subject"] != "new" {
		t.Errorf("diff mismatch: before=%v after=%v", before, after)
	}
}

func TestLog_Delete(t *testing.T) {
	db := connectDB(t)
	prefix := "test_audit_delete_"
	cleanup(t, db, prefix)
	defer cleanup(t, db, prefix)

	ctx := WithActor(context.Background(), 3, "carol", "")
	target := prefix + time.Now().Format("150405")
	Log(ctx, db, ActionDelete, TargetEmailGroups, 42, target,
		map[string]string{"name": "deleted-group"}, nil)

	var rec model.AuditLog
	if err := db.Where("target_name = ?", target).First(&rec).Error; err != nil {
		t.Fatalf("未写入: %v", err)
	}
	if rec.Action != ActionDelete {
		t.Errorf("Action mismatch")
	}
	if len(rec.AfterValue) > 0 && string(rec.AfterValue) != "null" {
		t.Errorf("delete 时 after 应为空")
	}
}

func TestLog_NilCtx(t *testing.T) {
	db := connectDB(t)
	prefix := "test_audit_nilctx_"
	cleanup(t, db, prefix)
	defer cleanup(t, db, prefix)

	// ctx 没 actor 注入
	target := prefix + time.Now().Format("150405")
	Log(context.Background(), db, ActionUpdate, TargetMailSettings, 1, target,
		map[string]string{"k": "v1"}, map[string]string{"k": "v2"})

	var rec model.AuditLog
	if err := db.Where("target_name = ?", target).First(&rec).Error; err != nil {
		t.Fatalf("未写入: %v", err)
	}
	if rec.UserId != nil {
		t.Errorf("UserId 应为 NULL, 实际 %v", *rec.UserId)
	}
	if rec.UserName != "" {
		t.Errorf("UserName 应为空, 实际 %s", rec.UserName)
	}
}

// TestLog_DBError 模拟 db 失败,验证 Log 不 panic 不阻塞
func TestLog_DBError(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过")
	}
	// 用一个已关闭/未连接的 db
	badDB, err := gorm.Open(mysql.Open("root:wrong-password@tcp(127.0.0.1:9)/none"),
		&gorm.Config{})
	if err == nil {
		// 关掉底层连接,让任何写入失败
		if sqlDB, e := badDB.DB(); e == nil {
			sqlDB.Close()
		}
	}
	// 即便 badDB 没真正连接成功,Log 也应该不 panic 不返回错误
	done := make(chan struct{})
	go func() {
		defer close(done)
		Log(context.Background(), badDB, ActionCreate, TargetUsers, 1, "test_db_err",
			nil, map[string]string{"k": "v"})
	}()
	select {
	case <-done:
		// good
	case <-time.After(5 * time.Second):
		t.Fatalf("Log 超时, 可能阻塞")
	}
}

func TestActorFromCtx(t *testing.T) {
	uid, uname, ip := ActorFromCtx(context.Background())
	if uid != 0 || uname != "" || ip != "" {
		t.Errorf("空 ctx 应返回零值, 实际 %d %s %s", uid, uname, ip)
	}
	ctx := WithActor(context.Background(), 7, "dave", "1.2.3.4")
	uid, uname, ip = ActorFromCtx(ctx)
	if uid != 7 || uname != "dave" || ip != "1.2.3.4" {
		t.Errorf("注入后取出不一致")
	}
}

// 防 audit_log 表名 typo
func TestTableName(t *testing.T) {
	a := model.AuditLog{}
	if a.TableName() != "audit_logs" {
		t.Errorf("表名应 audit_logs, 实际 %s", a.TableName())
	}
}

// 防止 IP 字段写超长
func TestLog_LongIp(t *testing.T) {
	db := connectDB(t)
	prefix := "test_audit_longip_"
	cleanup(t, db, prefix)
	defer cleanup(t, db, prefix)

	longIp := strings.Repeat("9", 100) // 超过 64 字节
	ctx := WithActor(context.Background(), 1, "x", longIp)
	target := prefix + time.Now().Format("150405")
	// 别炸
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = recover() }()
		Log(ctx, db, ActionCreate, TargetUsers, 1, target, nil, struct{ X string }{X: "y"})
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("超时")
	}
	// 长 IP 由 mysql 截断或报错,Log 内部捕获错误不抛出
}

// 防回归:Log 永远不返回 error 类型
func TestLogSignature(t *testing.T) {
	// 编译期检查:Log 签名无返回 (无 error)
	var fn func(context.Context, *gorm.DB, string, string, int64, string, interface{}, interface{}) = Log
	_ = fn
	// 也校验空 db 不 panic
	if r := safeCall(func() { Log(context.Background(), nil, "x", "y", 1, "z", nil, nil) }); r != nil {
		// 期望 panic 被内部 recover 捕获,这里若仍 panic 则报错
		t.Errorf("Log 应内部 recover, 实际抛出 %v", r)
	}
}

func safeCall(fn func()) (recovered interface{}) {
	defer func() { recovered = recover() }()
	fn()
	return errors.New("no panic")
}
