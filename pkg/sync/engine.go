package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"meeting/internal/config"
	"meeting/internal/model"
	"meeting/pkg/cache"
	"meeting/pkg/dingtalk"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

type Engine struct {
	mu    sync.Mutex
	db    *gorm.DB
	rdb   *redis.Client
	sheet *dingtalk.SheetClient
	cfg   config.Config
}

func NewEngine(db *gorm.DB, rdb *redis.Client, sheet *dingtalk.SheetClient, cfg config.Config) *Engine {
	return &Engine{db: db, rdb: rdb, sheet: sheet, cfg: cfg}
}

// RunFullSync 执行全量同步，按依赖顺序
func (e *Engine) RunFullSync(ctx context.Context) error {
	if !e.mu.TryLock() {
		return fmt.Errorf("同步正在进行中")
	}
	defer e.mu.Unlock()

	start := time.Now()
	logx.Infof("[DataSync] 开始全量同步")

	var errs []string

	// ① 酒店基础信息 → 产出 recordId→hotelId 映射
	recordIdToHotelId, err := e.syncHotels(ctx)
	if err != nil {
		errs = append(errs, fmt.Sprintf("hotels: %v", err))
		e.logSync("hotels", "failed", 0, err.Error())
	}

	// ② 会议室 → 用映射解析 linkedRecordIds
	if err := e.syncVenues(ctx, recordIdToHotelId); err != nil {
		errs = append(errs, fmt.Sprintf("venues: %v", err))
		e.logSync("venues", "failed", 0, err.Error())
	}

	// ③ 酒店设施表（元数据丰富：星级/品牌/集团/地区 + 会议室面积/剧院式/有柱）
	if err := e.syncHotelFacilities(ctx); err != nil {
		errs = append(errs, fmt.Sprintf("hotel_facilities: %v", err))
		e.logSync("hotel_facilities", "failed", 0, err.Error())
	}

	// ④ 每日出租记录（Daily Data Input → meeting_records）
	if err := e.syncMeetingRecords(ctx); err != nil {
		errs = append(errs, fmt.Sprintf("meeting_records: %v", err))
		e.logSync("meeting_records", "failed", 0, err.Error())
	}

	// ⑤ 酒店活动明细（Hotel Event → hotel_events，供竞对活动明细展示）
	if err := e.syncHotelEvents(ctx); err != nil {
		errs = append(errs, fmt.Sprintf("hotel_events: %v", err))
		e.logSync("hotel_events", "failed", 0, err.Error())
	}

	// ⑥ 用户权限（酒店对接人员）
	if err := e.syncUserPerms(ctx); err != nil {
		errs = append(errs, fmt.Sprintf("user_perms: %v", err))
		e.logSync("user_perms", "failed", 0, err.Error())
	}

	// ⑦ 城市活动
	if err := e.syncCityEvents(ctx); err != nil {
		errs = append(errs, fmt.Sprintf("city_events: %v", err))
		e.logSync("city_events", "failed", 0, err.Error())
	}

	// 清空 Redis 缓存
	cache.DeletePattern(ctx, e.rdb, "dashboard:*")
	cache.DeletePattern(ctx, e.rdb, "config:*")

	elapsed := time.Since(start)
	if len(errs) > 0 {
		msg := fmt.Sprintf("同步完成（耗时 %s），有错误: %v", elapsed, errs)
		logx.Errorf("[DataSync] %s", msg)
		e.logSync("full_sync", "failed", 0, msg)
		return fmt.Errorf(msg)
	}

	msg := fmt.Sprintf("全量同步完成，耗时 %s", elapsed)
	logx.Infof("[DataSync] %s", msg)
	e.logSync("full_sync", "success", 0, msg)
	return nil
}

func (e *Engine) logSync(source, status string, count int, message string) {
	e.db.Create(&model.SyncLog{
		Source:      source,
		Status:      status,
		RecordCount: count,
		Message:     message,
		SyncedAt:    time.Now(),
	})
}
