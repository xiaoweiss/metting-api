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

// notifyMissingHotels 在 batch 结束后,按 (hotelId, date) 聚合发钉钉群机器人提醒。
// Redis 24h dedupe:同 (hotelId, date) 已通知过则跳过。Redis 故障时降级直发(不阻塞主流程)。
func (e *Engine) notifyMissingHotels(ctx context.Context, missing *sync.Map) {
	missing.Range(func(k, v any) bool {
		key := k.(missingKey)
		count := v.(*atomic.Int64).Load()
		if count <= 0 {
			return true
		}
		if err := e.notifyOneMissing(ctx, key, count); err != nil {
			logx.Errorf("[Blast] 看板图缺失提醒失败 hotel=%d date=%s: %v", key.HotelId, key.Date, err)
		}
		return true
	})
}

func (e *Engine) notifyOneMissing(ctx context.Context, key missingKey, count int64) error {
	if e.Redis != nil {
		redisKey := fmt.Sprintf("notified:dashboard-missing:%d:%s", key.HotelId, key.Date)
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

	text := fmt.Sprintf(
		"⚠️ 看板图缺失,邮件已跳过\n酒店:%s\n日期:%s\n受影响邮件:%d 封\n请联系酒店对接人尽快在 PC 端点击「保存」生成今日看板图。",
		hotelName, key.Date, count,
	)
	sender := &notify.DingTalkRobotSender{DB: e.DB}
	return sender.Send(ctx, notify.Message{
		Title: "看板图缺失提醒",
		Text:  text,
	})
}
