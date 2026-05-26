package email

import (
	"context"
	"strconv"
	"strings"
	"time"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListEmailLogsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListEmailLogsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListEmailLogsLogic {
	return &ListEmailLogsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListEmailLogsLogic) ListEmailLogs(req *types.EmailLogListReq) (resp *types.EmailLogListResp, err error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}

	var total int64
	l.svcCtx.DB.Model(&model.EmailLog{}).Count(&total)

	var logs []model.EmailLog
	if err := l.svcCtx.DB.
		Order("id DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Find(&logs).Error; err != nil {
		return nil, err
	}

	// 一次性把模板名拉好（避免 N+1）
	tplIds := map[int64]struct{}{}
	groupIds := map[int64]struct{}{}
	for _, lg := range logs {
		if lg.TemplateId > 0 {
			tplIds[lg.TemplateId] = struct{}{}
		}
		if id, _, ok := parseGroupIdFromSource(lg.Source); ok {
			groupIds[id] = struct{}{}
		}
	}
	tplName := map[int64]string{}
	if len(tplIds) > 0 {
		var tpls []model.MailTemplate
		ids := make([]int64, 0, len(tplIds))
		for id := range tplIds {
			ids = append(ids, id)
		}
		l.svcCtx.DB.Where("id IN ?", ids).Find(&tpls)
		for _, t := range tpls {
			tplName[t.Id] = t.Name
		}
	}
	groupName := map[int64]string{}
	if len(groupIds) > 0 {
		var groups []model.EmailGroup
		ids := make([]int64, 0, len(groupIds))
		for id := range groupIds {
			ids = append(ids, id)
		}
		l.svcCtx.DB.Where("id IN ?", ids).Find(&groups)
		for _, g := range groups {
			groupName[g.Id] = g.Name
		}
	}

	resp = &types.EmailLogListResp{Total: total}
	for _, lg := range logs {
		item := types.EmailLogItem{
			Id:           lg.Id,
			ScheduleId:   lg.ScheduleId,
			TemplateId:   lg.TemplateId,
			TemplateName: tplName[lg.TemplateId],
			Source:       lg.Source,
			SourceLabel:  sourceLabel(lg.Source, groupName),
			Status:       lg.Status,
			Total:        lg.Total,
			FailCount:    lg.FailCount,
			RetryCount:   lg.RetryCount,
			SentAt:       lg.SentAt.Format(time.RFC3339),
		}
		resp.List = append(resp.List, item)
	}
	if resp.List == nil {
		resp.List = []types.EmailLogItem{}
	}
	return resp, nil
}

// parseGroupIdFromSource 解析 source。
// 新格式: "group:7|苏州万豪"      (多酒店,每家一封)
// 旧格式: "group:7"               (单酒店,迁移前的历史日志)
// 返回 (groupId, hotelName, ok)
func parseGroupIdFromSource(src string) (int64, string, bool) {
	if !strings.HasPrefix(src, "group:") {
		return 0, "", false
	}
	body := strings.TrimPrefix(src, "group:")
	hotelName := ""
	if i := strings.Index(body, "|"); i >= 0 {
		hotelName = body[i+1:]
		body = body[:i]
	}
	id, err := strconv.ParseInt(body, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return id, hotelName, true
}

func sourceLabel(src string, groupName map[int64]string) string {
	switch {
	case src == "blast":
		return "全员群发"
	case src == "legacy":
		return "历史日志"
	case src == "manual":
		return "手动发送"
	case strings.HasPrefix(src, "group:"):
		if id, hotelName, ok := parseGroupIdFromSource(src); ok {
			gn := groupName[id]
			if gn == "" {
				gn = "邮件组 #" + strconv.FormatInt(id, 10)
			} else {
				gn = "邮件组：" + gn
			}
			if hotelName != "" {
				return gn + " · " + hotelName
			}
			return gn
		}
	case strings.HasPrefix(src, "retry:"):
		return "重发 #" + strings.TrimPrefix(src, "retry:")
	}
	if src == "" {
		return "—"
	}
	return src
}
