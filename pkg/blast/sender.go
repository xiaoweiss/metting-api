// Package blast 全员群发邮件 —— 拿到模板 + 收件人列表 → goroutine pool 并发发 → 落 email_logs
package blast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"meeting/internal/config"
	"meeting/internal/model"
	"meeting/pkg/mail"

	"github.com/zeromicro/go-zero/core/logx"
)

// 并发度：同时打开的 SMTP 连接数。阿里企业邮服务端限流大概在 10/s 上下，
// 所以并发不要拉太高，4-8 比较稳。
const concurrency = 6

// 防抖：cron 抖动 / 进程刚启动立刻触发的情况下，
// 同一个 schedule 在 minDebounce 内已经跑过，就跳过。
const minDebounce = 30 * time.Second

type Engine struct {
	DB  *gorm.DB
	Cfg config.Config
}

func NewEngine(db *gorm.DB, cfg config.Config) *Engine {
	return &Engine{DB: db, Cfg: cfg}
}

// FailItem 单封发送失败的细节，会序列化进 email_logs.fail_list
type FailItem struct {
	Email string `json:"email"`
	Error string `json:"error"`
}

// RunBlast 执行一次全员群发：去重收件人 → 渲染模板 → goroutine pool 并发发 → 落日志
// 返回 EmailLog 记录（已写入 DB），调用方可读 status / fail_list 来呈现结果。
func (e *Engine) RunBlast(ctx context.Context) (*model.EmailLog, error) {
	// 1) 拉调度配置（包含 template_id 和 last_run_at）
	var sch model.MailBlastSchedule
	if err := e.DB.Where("lock_key = ?", "singleton").First(&sch).Error; err != nil {
		return nil, fmt.Errorf("查询群发调度失败: %w", err)
	}
	if sch.TemplateId == 0 {
		return nil, errors.New("未选择邮件模板")
	}

	// 2) 防抖：30 秒内已经跑过 → 跳过（cron 触发后再被人手动 trigger 一次就跳过）
	now := time.Now()
	if sch.LastRunAt != nil && now.Sub(*sch.LastRunAt) < minDebounce {
		logx.Infof("[Blast] 防抖：上次发送 %s，距今 %s 不足 %s，跳过",
			sch.LastRunAt.Format(time.RFC3339), now.Sub(*sch.LastRunAt), minDebounce)
		return nil, fmt.Errorf("距上次发送不足 %s，已跳过", minDebounce)
	}

	// 3) 加载模板
	var tpl model.MailTemplate
	if err := e.DB.First(&tpl, sch.TemplateId).Error; err != nil {
		return nil, fmt.Errorf("模板不存在 (id=%d): %w", sch.TemplateId, err)
	}

	// 4) 渲染主题 + 正文（用通用变量；目前先传 .Date / .Time）
	vars := map[string]interface{}{
		"Date": now.Format("2006-01-02"),
		"Time": now.Format("15:04"),
	}
	subject, body, err := mail.RenderSubjectAndBody(tpl.Subject, tpl.Body, vars)
	if err != nil {
		return nil, fmt.Errorf("模板渲染失败: %w", err)
	}

	// 5) 拉收件人 —— users.email 去重 + active
	var emails []string
	e.DB.Raw(`
		SELECT DISTINCT email FROM users
		WHERE email <> '' AND status = 'active'
	`).Scan(&emails)
	emails = dedup(emails) // 双保险，DISTINCT 已经做过

	if len(emails) == 0 {
		return nil, errors.New("没有可投递的收件人（users.email 全为空）")
	}

	// 6) 抢占 last_run_at —— 在真正开始前就把 last_run_at 写上，
	//    这样如果别的地方同时触发，能立刻被防抖挡掉
	e.DB.Model(&model.MailBlastSchedule{}).
		Where("lock_key = ?", "singleton").
		Update("last_run_at", now)

	// 7) goroutine pool 并发发
	mailer := mail.NewSender(e.DB, e.Cfg)
	var (
		failMu  sync.Mutex
		fails   []FailItem
		okCount int64
		sem     = make(chan struct{}, concurrency)
		wg      sync.WaitGroup
	)
	for _, addr := range emails {
		addr := addr
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			if err := mailer.Send([]string{addr}, subject, body); err != nil {
				failMu.Lock()
				fails = append(fails, FailItem{Email: addr, Error: err.Error()})
				failMu.Unlock()
				return
			}
			atomic.AddInt64(&okCount, 1)
		}()
	}
	wg.Wait()

	// 8) 落 email_logs
	status := "success"
	if len(fails) > 0 {
		if int(okCount) == 0 {
			status = "failed"
		} else {
			status = "partial"
		}
	}
	failJSON, _ := json.Marshal(fails)
	logRow := model.EmailLog{
		ScheduleId: sch.Id,
		Status:     status,
		Total:      len(emails),
		FailCount:  len(fails),
		FailList:   string(failJSON),
		SentAt:     now,
		CreatedAt:  now,
	}
	if err := e.DB.Create(&logRow).Error; err != nil {
		logx.Errorf("[Blast] 写 email_logs 失败: %v", err)
	}

	logx.Infof("[Blast] 全员群发完成：总 %d / 成功 %d / 失败 %d, status=%s",
		len(emails), okCount, len(fails), status)
	return &logRow, nil
}

func dedup(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
