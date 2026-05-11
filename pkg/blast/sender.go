// Package blast 群发邮件 —— 全员定时群发 + 邮件组按需触发，
// 共享同一套并发发送 + 落日志的代码。
package blast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
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
	DB    *gorm.DB
	Cfg   config.Config
	Redis *redis.Client // 用于看板图缺失提醒的 24h dedupe;可空,空时降级直发
}

func NewEngine(db *gorm.DB, rdb *redis.Client, cfg config.Config) *Engine {
	return &Engine{DB: db, Redis: rdb, Cfg: cfg}
}

// FailItem 单封发送失败的细节，会序列化进 email_logs.fail_list
type FailItem struct {
	Email string `json:"email"`
	Error string `json:"error"`
}

// missingKey 看板图缺失场景下,(酒店, 日期, 类型) 维度聚合受影响 recipient 数量
// Kind: "image" / "pdf" / "both" —— 用于钉钉提醒文案区分
type missingKey struct {
	HotelId int64
	Date    string
	Kind    string
}

// sendBatch 给一组邮箱并发发模板邮件 + 落 email_logs。
// 调用方负责传 emails 和 templateId；scheduleId 用来在 email_logs 里标记是哪个调度触发的（手动触发可传 0）。
// source 只是日志前缀，用于区分调用来源。
// hotelOverride > 0 时，所有收件人都按这家酒店渲染（邮件组发送 = group.hotel_id）；否则按各自 user.primary_hotel_id 决定。
//
// 模板里使用了 {{.DashboardImage}} 但当日 snapshot 缺失时,该 recipient 会被跳过(status=skipped),
// 不阻塞其它 recipient;批结束后按 (酒店, 日期) 聚合一次钉钉群机器人提醒。
func (e *Engine) sendBatch(ctx context.Context, emails []string, templateId, scheduleId int64, source string, hotelOverride int64) (*model.EmailLog, error) {
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

	// 加载模板静态附件(per-template,所有收件人共享)
	tplInlineImages, tplAttachments := e.loadTemplateAttachments(templateId)

	// 单封结果（顺序与 emails 对齐）
	type result struct {
		Email   string
		Err     string
		Skipped bool
	}
	results := make([]result, len(emails))

	// 看板缺失聚合: (hotelId, date, kind) → 受影响 recipient 数
	missing := sync.Map{}

	// 是否需要 snapshot:模板里用到对应变量才把 missing 当做 skip 处理。
	// 没用变量的模板,即便 snapshot 找不到也照常发(向后兼容)。
	needImage := strings.Contains(tpl.Body, "{{.DashboardImage}}") || strings.Contains(tpl.Subject, "{{.DashboardImage}}")
	needPDF := strings.Contains(tpl.Body, "{{.DashboardPDF}}") || strings.Contains(tpl.Subject, "{{.DashboardPDF}}")

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
			vars, inlineImages, perRecipAtts, pngFound, pdfFound, hotelId := recipientVars(e.DB, e.Cfg, addr, now, hotelOverride)

			// 计算缺失:模板真的引用了 + 服务器没找到 → skip + 聚合钉钉提醒
			imgMiss := needImage && !pngFound
			pdfMiss := needPDF && !pdfFound
			if imgMiss || pdfMiss {
				dateStr, _ := vars["Date"].(string)
				kind := "image"
				switch {
				case imgMiss && pdfMiss:
					kind = "both"
				case pdfMiss:
					kind = "pdf"
				}
				results[i].Skipped = true
				results[i].Err = fmt.Sprintf("dashboard %s missing for hotel=%d date=%s", kind, hotelId, dateStr)
				k := missingKey{HotelId: hotelId, Date: dateStr, Kind: kind}
				v, _ := missing.LoadOrStore(k, new(atomic.Int64))
				v.(*atomic.Int64).Add(1)
				return
			}

			subject, body, rerr := mail.RenderSubjectAndBody(tpl.Subject, tpl.Body, vars)
			if rerr != nil {
				results[i].Err = "模板渲染失败: " + rerr.Error()
				return
			}
			// per-recipient 资源按模板引用做过滤:模板没引用变量就不附进邮件,避免"附 PDF 时 PNG 也搭便车发"
			effInline := []mail.InlineImage(nil)
			if needImage {
				effInline = inlineImages
			}
			effAtts := []mail.Attachment(nil)
			if needPDF {
				effAtts = perRecipAtts
			}
			// 模板级静态附件不受变量约束(那是用户在编辑器里手动挂的)
			mergedInline := append(append([]mail.InlineImage(nil), effInline...), tplInlineImages...)
			mergedAtts := append(append([]mail.Attachment(nil), effAtts...), tplAttachments...)
			logx.Infof("[Blast.send] to=%s tpl_id=%d source=%s needImage=%v needPDF=%v → final inline=%d att=%d (perRecipPng=%d perRecipPdf=%d tplInline=%d tplAtt=%d)",
				addr, templateId, source, needImage, needPDF, len(mergedInline), len(mergedAtts),
				len(inlineImages), len(perRecipAtts), len(tplInlineImages), len(tplAttachments))
			for _, im := range mergedInline {
				logx.Infof("[Blast.send]   inline: %s", im.FilePath)
			}
			for _, at := range mergedAtts {
				logx.Infof("[Blast.send]   attach: %s (%s)", at.FilePath, at.Filename)
			}
			if err := mailer.Send([]string{addr}, subject, body, mergedInline, mergedAtts); err != nil {
				results[i].Err = err.Error()
				return
			}
			atomic.AddInt64(&okCount, 1)
		}()
	}
	wg.Wait()

	// batch 结束:对 (hotelId, date) 维度的 missing 触发钉钉群机器人提醒(下次 task 8 wire-in)
	e.notifyMissingHotels(ctx, &missing)

	// 汇总失败列表（兼容老 fail_list）+ 准备 recipients 行
	var fails []FailItem
	for _, r := range results {
		if r.Err != "" {
			fails = append(fails, FailItem{Email: r.Email, Error: r.Err})
		}
	}

	// "skipped" 不计入失败 fail_list,但走 partial/failed 状态判断时算"未成功"
	skippedCount := 0
	for _, r := range results {
		if r.Skipped {
			skippedCount++
		}
	}

	status := "success"
	if len(fails) > 0 || skippedCount > 0 {
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
			if r.Skipped {
				st = "skipped"
			} else if r.Err != "" {
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

// RunBlastById 执行某条 schedule 的全员群发(cron 触发或手动触发都走这里)。
//   - 按 schedule_id 拉配置
//   - 30 秒防抖
//   - 收件人 = users 里所有 active 且填了邮箱的人
func (e *Engine) RunBlastById(ctx context.Context, scheduleId int64) (*model.EmailLog, error) {
	var sch model.MailBlastSchedule
	if err := e.DB.First(&sch, scheduleId).Error; err != nil {
		return nil, fmt.Errorf("查询群发调度 id=%d 失败: %w", scheduleId, err)
	}
	if sch.TemplateId == 0 {
		return nil, errors.New("未选择邮件模板")
	}

	now := time.Now()
	if sch.LastRunAt != nil && now.Sub(*sch.LastRunAt) < minDebounce {
		logx.Infof("[Blast] schedule=%d 防抖:上次 %s,距今 %s 不足 %s,跳过",
			scheduleId, sch.LastRunAt.Format(time.RFC3339), now.Sub(*sch.LastRunAt), minDebounce)
		return nil, fmt.Errorf("距上次发送不足 %s,已跳过", minDebounce)
	}

	var emails []string
	e.DB.Raw(`
		SELECT DISTINCT email FROM users
		WHERE email <> '' AND status = 'active'
	`).Scan(&emails)
	if len(dedup(emails)) == 0 {
		return nil, errors.New("没有可投递的收件人(users.email 全为空)")
	}

	// 抢占 last_run_at —— 真正发送前先写,避免并发时两个调度都过了防抖检查
	e.DB.Model(&model.MailBlastSchedule{}).
		Where("id = ?", scheduleId).
		Update("last_run_at", now)

	source := fmt.Sprintf("blast:%d", scheduleId)
	if sch.Name != "" {
		source = fmt.Sprintf("blast:%d(%s)", scheduleId, sch.Name)
	}
	return e.sendBatch(ctx, emails, sch.TemplateId, sch.Id, source, 0)
}

// SendGroup 给一个邮件组的所有成员发模板邮件，立即触发（不走 cron）。
// 用 group.hotel_id 作为模板渲染的对标酒店（覆盖收件人各自的 primary_hotel_id），
// 这样"金鸡湖万豪对接组"发出的邮件不论谁收到，HotelName / 出租率 都是金鸡湖万豪的。
// 异步：调用方在 goroutine 里发，但本函数本身是同步的（每封邮件内部并发）。
func (e *Engine) SendGroup(ctx context.Context, groupId, templateId int64) (*model.EmailLog, error) {
	var grp model.EmailGroup
	if err := e.DB.First(&grp, groupId).Error; err != nil {
		return nil, fmt.Errorf("邮件组不存在: %w", err)
	}
	// 加载组成员的邮箱
	var emails []string
	e.DB.Raw(`
		SELECT DISTINCT email FROM email_group_members
		WHERE group_id = ? AND email <> ''
	`, groupId).Scan(&emails)
	if len(dedup(emails)) == 0 {
		return nil, errors.New("该邮件组没有可投递的成员")
	}
	return e.sendBatch(ctx, emails, templateId, 0, fmt.Sprintf("group:%d", groupId), grp.HotelId)
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

	// 重发不知道原始 hotelOverride（email_logs 没记），按收件人 primary_hotel_id 走
	return e.sendBatch(ctx, emails, src.TemplateId, src.ScheduleId, fmt.Sprintf("retry:%d", src.Id), 0)
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
	return e.sendBatch(ctx, []string{rec.Email}, src.TemplateId, src.ScheduleId, fmt.Sprintf("retry:%d", src.Id), 0)
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

// loadTemplateAttachments 拉模板挂载的静态附件并按 mime 分流:
// 图片 (image/*) 进 inlineImages(Embed,cid 可被 HTML 引用);其它(PDF / 文档)进 attachments(Attach 附件区)。
func (e *Engine) loadTemplateAttachments(templateId int64) ([]mail.InlineImage, []mail.Attachment) {
	var rows []model.MailTemplateAttachment
	if err := e.DB.Where("template_id = ?", templateId).Order("sort_order, id").Find(&rows).Error; err != nil {
		logx.Errorf("[Blast] 加载模板附件失败 templateId=%d: %v", templateId, err)
		return nil, nil
	}
	var inlines []mail.InlineImage
	var atts []mail.Attachment
	for _, r := range rows {
		abs := filepath.Join(e.Cfg.Mail.AttachmentDir, r.FilePath)
		if strings.HasPrefix(r.MimeType, "image/") {
			inlines = append(inlines, mail.InlineImage{FilePath: abs})
		} else {
			atts = append(atts, mail.Attachment{FilePath: abs, Filename: r.OriginalName})
		}
	}
	return inlines, atts
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
