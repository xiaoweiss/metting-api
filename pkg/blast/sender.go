// Package blast 群发邮件 —— 全员定时群发 + 邮件组按需触发，
// 共享同一套并发发送 + 落日志的代码。
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

// sendBatch 给一组邮箱并发发模板邮件 + 落 email_logs。
// 调用方负责传 emails 和 templateId；scheduleId 用来在 email_logs 里标记是哪个调度触发的（手动触发可传 0）。
// source 只是日志前缀，用于区分调用来源。
func (e *Engine) sendBatch(_ context.Context, emails []string, templateId, scheduleId int64, source string) (*model.EmailLog, error) {
	if len(emails) == 0 {
		return nil, errors.New("收件人为空")
	}
	if templateId == 0 {
		return nil, errors.New("未选择邮件模板")
	}

	// 加载模板
	var tpl model.MailTemplate
	if err := e.DB.First(&tpl, templateId).Error; err != nil {
		return nil, fmt.Errorf("模板不存在 (id=%d): %w", templateId, err)
	}

	now := time.Now()
	emails = dedup(emails)
	mailer := mail.NewSender(e.DB, e.Cfg)
	// 不在这里预渲染 subject/body —— 每个收件人有自己的酒店/出租率，
	// 进 goroutine 后用 recipientVars 各自装配 + 渲染。

	// 单封结果（顺序与 emails 对齐）
	type result struct {
		Email string
		Err   string
	}
	results := make([]result, len(emails))

	var (
		okCount int64
		sem     = make(chan struct{}, concurrency)
		wg      sync.WaitGroup
	)
	for i, addr := range emails {
		i, addr := i, addr
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			results[i].Email = addr
			vars := recipientVars(e.DB, addr, now)
			subject, body, rerr := mail.RenderSubjectAndBody(tpl.Subject, tpl.Body, vars)
			if rerr != nil {
				results[i].Err = "模板渲染失败: " + rerr.Error()
				return
			}
			if err := mailer.Send([]string{addr}, subject, body); err != nil {
				results[i].Err = err.Error()
				return
			}
			atomic.AddInt64(&okCount, 1)
		}()
	}
	wg.Wait()

	// 汇总失败列表（兼容老 fail_list）+ 准备 recipients 行
	var fails []FailItem
	for _, r := range results {
		if r.Err != "" {
			fails = append(fails, FailItem{Email: r.Email, Error: r.Err})
		}
	}

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
		ScheduleId: scheduleId,
		TemplateId: templateId,
		Source:     source,
		Status:     status,
		Total:      len(emails),
		FailCount:  len(fails),
		FailList:   string(failJSON),
		SentAt:     now,
		CreatedAt:  now,
	}
	if err := e.DB.Create(&logRow).Error; err != nil {
		logx.Errorf("[%s] 写 email_logs 失败: %v", source, err)
	}

	// 把 N 个收件人各落一行 email_log_recipients
	if logRow.Id > 0 {
		recipients := make([]model.EmailLogRecipient, 0, len(results))
		for _, r := range results {
			st := "sent"
			if r.Err != "" {
				st = "failed"
			}
			recipients = append(recipients, model.EmailLogRecipient{
				LogId:     logRow.Id,
				Email:     r.Email,
				Status:    st,
				Error:     r.Err,
				SentAt:    now,
				CreatedAt: now,
			})
		}
		if len(recipients) > 0 {
			// gorm CreateInBatches 100 一批，避免一条 SQL 过长
			if err := e.DB.CreateInBatches(recipients, 100).Error; err != nil {
				logx.Errorf("[%s] 写 email_log_recipients 失败: %v", source, err)
			}
		}
	}

	logx.Infof("[%s] 群发完成：总 %d / 成功 %d / 失败 %d, status=%s",
		source, len(emails), okCount, len(fails), status)
	return &logRow, nil
}

// RunBlast 执行一次全员群发（cron 触发或手动触发都走这里）：
//   - 拉 mail_blast_schedules.singleton 配置
//   - 30 秒防抖
//   - 收件人 = users 里所有 active 且填了邮箱的人
func (e *Engine) RunBlast(ctx context.Context) (*model.EmailLog, error) {
	var sch model.MailBlastSchedule
	if err := e.DB.Where("lock_key = ?", "singleton").First(&sch).Error; err != nil {
		return nil, fmt.Errorf("查询群发调度失败: %w", err)
	}
	if sch.TemplateId == 0 {
		return nil, errors.New("未选择邮件模板")
	}

	now := time.Now()
	if sch.LastRunAt != nil && now.Sub(*sch.LastRunAt) < minDebounce {
		logx.Infof("[Blast] 防抖：上次发送 %s，距今 %s 不足 %s，跳过",
			sch.LastRunAt.Format(time.RFC3339), now.Sub(*sch.LastRunAt), minDebounce)
		return nil, fmt.Errorf("距上次发送不足 %s，已跳过", minDebounce)
	}

	var emails []string
	e.DB.Raw(`
		SELECT DISTINCT email FROM users
		WHERE email <> '' AND status = 'active'
	`).Scan(&emails)
	if len(dedup(emails)) == 0 {
		return nil, errors.New("没有可投递的收件人（users.email 全为空）")
	}

	// 抢占 last_run_at —— 真正发送前先写，避免并发时两个调度都过了防抖检查
	e.DB.Model(&model.MailBlastSchedule{}).
		Where("lock_key = ?", "singleton").
		Update("last_run_at", now)

	return e.sendBatch(ctx, emails, sch.TemplateId, sch.Id, "blast")
}

// SendGroup 给一个邮件组的所有成员发模板邮件，立即触发（不走 cron）。
// 异步：调用方在 goroutine 里发，但本函数本身是同步的（每封邮件内部并发）。
// 在 handler 里要"返回快，发送在后台"的话，handler 自己再用 go func() 包一下。
func (e *Engine) SendGroup(ctx context.Context, groupId, templateId int64) (*model.EmailLog, error) {
	// 加载组成员的邮箱
	var emails []string
	e.DB.Raw(`
		SELECT DISTINCT email FROM email_group_members
		WHERE group_id = ? AND email <> ''
	`, groupId).Scan(&emails)
	if len(dedup(emails)) == 0 {
		return nil, errors.New("该邮件组没有可投递的成员")
	}
	return e.sendBatch(ctx, emails, templateId, 0, fmt.Sprintf("group:%d", groupId))
}

// RetryFailed 拿一条 email_logs，把它的 fail_list 里所有失败邮箱重发一遍。
// 用同一个 templateId（落在 email_logs.template_id），落新一行 email_logs。
// retryCount 在原始日志上 +1，方便前端展示重试次数。
func (e *Engine) RetryFailed(ctx context.Context, logId int64) (*model.EmailLog, error) {
	var src model.EmailLog
	if err := e.DB.First(&src, logId).Error; err != nil {
		return nil, fmt.Errorf("日志不存在: %w", err)
	}
	if src.TemplateId == 0 {
		return nil, errors.New("该日志缺少 template_id（可能是迁移前的历史日志），无法自动重发")
	}
	if src.FailList == "" || src.FailList == "[]" || src.FailList == "null" {
		return nil, errors.New("该日志没有失败明细，无需重发")
	}
	var items []FailItem
	if err := json.Unmarshal([]byte(src.FailList), &items); err != nil {
		return nil, fmt.Errorf("解析 fail_list 失败: %w", err)
	}
	emails := make([]string, 0, len(items))
	for _, it := range items {
		if it.Email != "" {
			emails = append(emails, it.Email)
		}
	}
	if len(emails) == 0 {
		return nil, errors.New("没有可重发的失败邮箱")
	}
	// 原始日志的重试计数 +1（即使新发也失败了，也代表用户做过一次重发动作）
	e.DB.Model(&model.EmailLog{}).
		Where("id = ?", src.Id).
		UpdateColumn("retry_count", gorm.Expr("retry_count + 1"))

	return e.sendBatch(ctx, emails, src.TemplateId, src.ScheduleId, fmt.Sprintf("retry:%d", src.Id))
}

// RetryRecipient 只重发 email_log_recipients 表里某一行（单封）。
// 走 sendBatch 落新一行 email_logs（total=1），原 recipient 行的 retry_count + 1。
func (e *Engine) RetryRecipient(ctx context.Context, recipientId int64) (*model.EmailLog, error) {
	var rec model.EmailLogRecipient
	if err := e.DB.First(&rec, recipientId).Error; err != nil {
		return nil, fmt.Errorf("收件人记录不存在: %w", err)
	}
	if rec.Email == "" {
		return nil, errors.New("收件人邮箱为空")
	}
	var src model.EmailLog
	if err := e.DB.First(&src, rec.LogId).Error; err != nil {
		return nil, fmt.Errorf("母日志不存在: %w", err)
	}
	if src.TemplateId == 0 {
		return nil, errors.New("该日志缺少 template_id（历史日志），无法重发")
	}
	// 原 recipient 计 retry+1
	e.DB.Model(&model.EmailLogRecipient{}).
		Where("id = ?", rec.Id).
		UpdateColumn("retry_count", gorm.Expr("retry_count + 1"))
	return e.sendBatch(ctx, []string{rec.Email}, src.TemplateId, src.ScheduleId, fmt.Sprintf("retry:%d", src.Id))
}

// RetryAllFailed 重发所有 status in (partial, failed) 的日志的失败明细。
// 串行处理每条日志（每条内部已经 goroutine 并发）。返回每条结果。
func (e *Engine) RetryAllFailed(ctx context.Context) ([]*model.EmailLog, error) {
	var srcLogs []model.EmailLog
	e.DB.Where("status IN ('partial','failed') AND template_id > 0 AND fail_count > 0").
		Order("id DESC").
		Find(&srcLogs)
	if len(srcLogs) == 0 {
		return nil, errors.New("没有可重发的失败日志")
	}
	results := make([]*model.EmailLog, 0, len(srcLogs))
	for _, s := range srcLogs {
		newLog, err := e.RetryFailed(ctx, s.Id)
		if err != nil {
			logx.Errorf("[RetryAll] log_id=%d 重发失败: %v", s.Id, err)
			continue
		}
		results = append(results, newLog)
	}
	return results, nil
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
