// Package audit 后台管理操作审计 (migration 017)
// 任何 admin write logic 通过 audit.Log(...) 异步落库,失败不阻塞业务。
// actor (操作人 user_id / user_name / ip) 由 AdminOnly middleware 注入 ctx,
// 下游 logic 无需操心 — 直接调用 audit.Log。
package audit

import (
	"context"
	"encoding/json"

	"meeting/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// 业务动作常量(写入 audit_logs.action 列)
const (
	ActionCreate  = "create"
	ActionUpdate  = "update"
	ActionDelete  = "delete"
	ActionTrigger = "trigger"
)

// 业务对象类型常量(写入 audit_logs.target_type 列)
const (
	TargetUsers              = "users"
	TargetMailSettings       = "mail_settings"
	TargetEmailGroups        = "email_groups"
	TargetEmailGroupMembers  = "email_group_members"
	TargetMailTemplates      = "mail_templates"
	TargetMailBlastSchedules = "mail_blast_schedules"
)

// ctx keys (middleware 注入)
type ctxKey int

const (
	ctxKeyUserId ctxKey = iota
	ctxKeyUserName
	ctxKeyIp
)

// WithActor 把操作人信息注入 ctx,middleware 调用一次,下游 logic 通过 audit.Log 隐式读取。
// userId == 0 视为 system,落库时 user_id = NULL。
func WithActor(ctx context.Context, userId int64, userName, ip string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, ctxKeyUserId, userId)
	ctx = context.WithValue(ctx, ctxKeyUserName, userName)
	ctx = context.WithValue(ctx, ctxKeyIp, ip)
	return ctx
}

// ActorFromCtx 从 ctx 取出 actor,无注入则返回 0, "", ""。
func ActorFromCtx(ctx context.Context) (userId int64, userName, ip string) {
	if ctx == nil {
		return 0, "", ""
	}
	if v, ok := ctx.Value(ctxKeyUserId).(int64); ok {
		userId = v
	}
	if v, ok := ctx.Value(ctxKeyUserName).(string); ok {
		userName = v
	}
	if v, ok := ctx.Value(ctxKeyIp).(string); ok {
		ip = v
	}
	return
}

// Log 写一条审计记录。
// 失败只 logx.Errorf,不返回 error,不 panic — 审计不允许阻塞业务。
// before / after 可为 nil:create 时 before=nil,delete 时 after=nil,trigger 通常两者都填(触发前后状态)。
func Log(ctx context.Context, db *gorm.DB, action, targetType string,
	targetId int64, targetName string, before, after interface{}) {

	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("[audit] panic recovered: action=%s target=%s/%d r=%v",
				action, targetType, targetId, r)
		}
	}()

	uid, uname, ip := ActorFromCtx(ctx)

	var beforeJSON, afterJSON datatypes.JSON
	if before != nil {
		if b, err := json.Marshal(before); err == nil {
			beforeJSON = b
		} else {
			logx.Errorf("[audit] marshal before failed: %v", err)
		}
	}
	if after != nil {
		if a, err := json.Marshal(after); err == nil {
			afterJSON = a
		} else {
			logx.Errorf("[audit] marshal after failed: %v", err)
		}
	}

	var userIdPtr *int64
	if uid > 0 {
		userIdPtr = &uid
	}
	var targetIdPtr *int64
	if targetId > 0 {
		targetIdPtr = &targetId
	}

	record := model.AuditLog{
		UserId:      userIdPtr,
		UserName:    uname,
		Action:      action,
		TargetType:  targetType,
		TargetId:    targetIdPtr,
		TargetName:  targetName,
		BeforeValue: beforeJSON,
		AfterValue:  afterJSON,
		Ip:          ip,
	}
	if err := db.Create(&record).Error; err != nil {
		logx.Errorf("[audit] write failed: action=%s target=%s/%d err=%v",
			action, targetType, targetId, err)
	}
}
