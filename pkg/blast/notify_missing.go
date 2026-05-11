package blast

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zeromicro/go-zero/core/logx"

	"meeting/pkg/notify"
)

// notifyMissingHotels 在 batch 结束后,按 (hotelId, date, kind) 聚合发钉钉群机器人提醒。
// Redis 24h dedupe:同 (hotelId, date, kind) 已通知过则跳过。Redis 故障时降级直发(不阻塞主流程)。
func (e *Engine) notifyMissingHotels(ctx context.Context, missing *sync.Map) {
	missing.Range(func(k, v any) bool {
		key := k.(missingKey)
		count := v.(*atomic.Int64).Load()
		if count <= 0 {
			return true
		}
		if err := e.notifyOneMissing(ctx, key, count); err != nil {
			logx.Errorf("[Blast] 看板缺失提醒失败 hotel=%d date=%s kind=%s: %v", key.HotelId, key.Date, key.Kind, err)
		}
		return true
	})
}

// kindLabel 把内部 kind 字符串转成中文提示
func kindLabel(kind string) (label, fileHint string) {
	switch kind {
	case "pdf":
		return "看板 PDF", "PDF"
	case "both":
		return "看板图 + PDF", "PNG + PDF"
	default:
		return "看板图", "PNG"
	}
}

func (e *Engine) notifyOneMissing(ctx context.Context, key missingKey, count int64) error {
	if e.Redis != nil {
		redisKey := fmt.Sprintf("notified:dashboard-missing:%d:%s:%s", key.HotelId, key.Date, key.Kind)
		ok, err := e.Redis.SetNX(ctx, redisKey, "1", 24*time.Hour).Result()
		if err == nil && !ok {
			// 24h 内已通知过,跳过
			return nil
		}
		// err != nil 时降级直发,不阻塞主流程
	}

	var hotelName string
	e.DB.Raw("SELECT name FROM hotels WHERE id = ?", key.HotelId).Scan(&hotelName)
	if hotelName == "" {
		hotelName = fmt.Sprintf("hotel #%d", key.HotelId)
	}

	missLabel, fileHint := kindLabel(key.Kind)
	text := fmt.Sprintf(
		"⚠️ %s 缺失,邮件已跳过\n酒店:%s\n日期:%s\n受影响邮件:%d 封\n请联系酒店对接人在 PC 端点击「保存 → %s」生成当日文件。",
		missLabel, hotelName, key.Date, count, fileHint,
	)
	sender := &notify.DingTalkRobotSender{DB: e.DB}
	return sender.Send(ctx, notify.Message{
		Title: missLabel + " 缺失提醒",
		Text:  text,
	})
}
