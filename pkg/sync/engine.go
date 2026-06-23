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
	mu                  sync.Mutex
	db                  *gorm.DB
	rdb                 *redis.Client
	sheet               *dingtalk.SheetClient
	cfg                 config.Config
	consecutiveFailures int       // 连续失败次数(含 hotels 0 家早 return)
	nextRunAfter        time.Time // 下次可跑的时间;zero 表示无 backoff;并发安全由 mu 保护
}

func NewEngine(db *gorm.DB, rdb *redis.Client, sheet *dingtalk.SheetClient, cfg config.Config) *Engine {
	return &Engine{db: db, rdb: rdb, sheet: sheet, cfg: cfg}
}

// RunFullSync 执行全量同步，按依赖顺序
// 失败时指数退避(1m / 2m / 4m / 8m / 16m / 30m 上限),避免持续 spam log + 浪费资源
func (e *Engine) RunFullSync(ctx context.Context) error {
	if !e.mu.TryLock() {
		return fmt.Errorf("同步正在进行中")
	}
	defer e.mu.Unlock()

	// backoff 中,silent skip(不打 error log,几乎 0 开销)
	if !e.nextRunAfter.IsZero() && time.Now().Before(e.nextRunAfter) {
		return nil
	}

	start := time.Now()
	logx.Infof("[DataSync] 开始全量同步")

	var errs []string

	// ① 酒店基础信息 → 产出 recordId→hotelId 映射
	recordIdToHotelId, err := e.syncHotels(ctx)
	if err != nil {
		errs = append(errs, fmt.Sprintf("hotels: %v", err))
		e.logSync("hotels", "failed", 0, err.Error())
	}

	// 早 return: hotels 0 家时,后续 6 个 sync 都依赖 hotel 数据,跑下去都是空操作,
	// 还会刷大量 log + 写 sync_logs 表 + 浪费 CPU。直接跳过本轮等下次,等钉钉源恢复
	if len(recordIdToHotelId) == 0 {
		msg := "hotels 同步 0 家(钉钉源数据异常或字段名变了),本轮跳过后续 sync"
		logx.Errorf("[DataSync] %s", msg)
		e.logSync("full_sync", "failed", 0, msg)
		e.bumpBackoffLocked(true)
		return fmt.Errorf("%s", msg)
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
		e.bumpBackoffLocked(true)
		return fmt.Errorf("%s", msg)
	}

	msg := fmt.Sprintf("全量同步完成，耗时 %s", elapsed)
	logx.Infof("[DataSync] %s", msg)
	e.logSync("full_sync", "success", 0, msg)
	e.bumpBackoffLocked(false)
	return nil
}

// bumpBackoffLocked 根据本次结果更新 backoff 状态;必须在 mu 持有时调用。
// 失败: 指数退避 1m / 2m / 4m / 8m / 16m / 30m,上限 30 分钟。
// 成功: 立刻清零,下次 cron 正常跑。
func (e *Engine) bumpBackoffLocked(failed bool) {
	if !failed {
		if e.consecutiveFailures > 0 {
			logx.Infof("[DataSync] 同步恢复,清 backoff(之前连续失败 %d 次)", e.consecutiveFailures)
		}
		e.consecutiveFailures = 0
		e.nextRunAfter = time.Time{}
		return
	}

	e.consecutiveFailures++
	// 指数退避: 1<<0=1, 1<<1=2, ..., 1<<5=32 → 上限取 30 分钟
	shift := uint(e.consecutiveFailures - 1)
	if shift > 5 {
		shift = 5
	}
	delay := time.Minute << shift
	if delay > 30*time.Minute {
		delay = 30 * time.Minute
	}
	e.nextRunAfter = time.Now().Add(delay)
	logx.Errorf("[DataSync] 连续失败 %d 次,backoff %v,下次最早 %s",
		e.consecutiveFailures, delay, e.nextRunAfter.Format("15:04:05"))
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
