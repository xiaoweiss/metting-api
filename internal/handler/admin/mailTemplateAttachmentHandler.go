package admin

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zeromicro/go-zero/rest/httpx"
	"gorm.io/gorm"

	"meeting/internal/model"
	"meeting/internal/svc"
)

const (
	maxAttachmentBytes      = 20 << 20 // 20MB per file
	maxAttachmentTotalBytes = 50 << 20 // 50MB total per template
	attachmentParseFormCap  = 21 << 20 // 21MB,留 1MB buffer 给 multipart 头
)

var safeCidRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// sanitizeCid 把原 basename 转成只含 [a-zA-Z0-9._-] 的字符,fallback 给个稳定名字。
func sanitizeCid(name string) string {
	name = filepath.Base(name)
	name = safeCidRe.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "att"
	}
	return name
}

// UploadMailTemplateAttachmentHandler POST /api/admin/mail-templates/:id/attachments
func UploadMailTemplateAttachmentHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Id int64 `path:"id"`
		}
		if err := httpx.Parse(r, &req); err != nil || req.Id == 0 {
			http.Error(w, "template id 非法", http.StatusBadRequest)
			return
		}

		// 校验模板存在
		var tpl model.MailTemplate
		if err := svcCtx.DB.First(&tpl, req.Id).Error; err != nil {
			http.Error(w, "模板不存在", http.StatusNotFound)
			return
		}

		if err := r.ParseMultipartForm(attachmentParseFormCap); err != nil {
			http.Error(w, "解析表单失败: "+err.Error(), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "缺少 file 字段", http.StatusBadRequest)
			return
		}
		defer file.Close()
		if header.Size > maxAttachmentBytes {
			http.Error(w, "文件超过 20MB 上限", http.StatusRequestEntityTooLarge)
			return
		}

		// 校验模板已用空间
		var usedBytes int64
		svcCtx.DB.Model(&model.MailTemplateAttachment{}).
			Where("template_id = ?", req.Id).
			Select("COALESCE(SUM(file_size), 0)").
			Scan(&usedBytes)
		if usedBytes+header.Size > maxAttachmentTotalBytes {
			http.Error(w, fmt.Sprintf("模板附件总和将超 50MB(已用 %d 字节)", usedBytes), http.StatusRequestEntityTooLarge)
			return
		}

		// 生成 cid + 文件路径
		baseCid := sanitizeCid(header.Filename)
		var existing []model.MailTemplateAttachment
		svcCtx.DB.Where("template_id = ?", req.Id).Find(&existing)
		used := map[string]bool{}
		for _, a := range existing {
			used[a.Cid] = true
		}
		cid := baseCid
		for i := 2; used[cid]; i++ {
			ext := filepath.Ext(baseCid)
			stem := strings.TrimSuffix(baseCid, ext)
			cid = fmt.Sprintf("%s-%d%s", stem, i, ext)
		}

		relPath := filepath.Join(fmt.Sprintf("template-%d", req.Id), cid)
		absPath := filepath.Join(svcCtx.Config.Mail.AttachmentDir, relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			http.Error(w, "创建目录失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		tmpPath := absPath + ".tmp"
		out, err := os.Create(tmpPath)
		if err != nil {
			http.Error(w, "写文件失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		written, copyErr := io.Copy(out, file)
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil {
			os.Remove(tmpPath)
			http.Error(w, "写文件失败", http.StatusInternalServerError)
			return
		}
		if err := os.Rename(tmpPath, absPath); err != nil {
			os.Remove(tmpPath)
			http.Error(w, "落盘失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		mime := header.Header.Get("Content-Type")
		if mime == "" {
			mime = "application/octet-stream"
		}

		row := model.MailTemplateAttachment{
			TemplateId:   req.Id,
			OriginalName: header.Filename,
			FilePath:     relPath,
			FileSize:     written,
			MimeType:     mime,
			Cid:          cid,
			SortOrder:    len(existing),
		}
		if err := svcCtx.DB.Create(&row).Error; err != nil {
			os.Remove(absPath)
			http.Error(w, "DB 写入失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		httpx.OkJsonCtx(r.Context(), w, map[string]any{
			"id":           row.Id,
			"originalName": row.OriginalName,
			"cid":          row.Cid,
			"size":         row.FileSize,
			"mimeType":     row.MimeType,
			"sortOrder":    row.SortOrder,
		})
	}
}

// ListMailTemplateAttachmentsHandler GET /api/admin/mail-templates/:id/attachments
func ListMailTemplateAttachmentsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Id int64 `path:"id"`
		}
		if err := httpx.Parse(r, &req); err != nil || req.Id == 0 {
			http.Error(w, "template id 非法", http.StatusBadRequest)
			return
		}
		var rows []model.MailTemplateAttachment
		svcCtx.DB.Where("template_id = ?", req.Id).Order("sort_order, id").Find(&rows)
		list := make([]map[string]any, 0, len(rows))
		for _, a := range rows {
			list = append(list, map[string]any{
				"id":           a.Id,
				"originalName": a.OriginalName,
				"cid":          a.Cid,
				"size":         a.FileSize,
				"mimeType":     a.MimeType,
				"sortOrder":    a.SortOrder,
				"uploadedAt":   a.UploadedAt,
			})
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]any{"list": list})
	}
}

// DeleteMailTemplateAttachmentHandler DELETE /api/admin/mail-templates/:id/attachments/:attId
func DeleteMailTemplateAttachmentHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Id    int64 `path:"id"`
			AttId int64 `path:"attId"`
		}
		if err := httpx.Parse(r, &req); err != nil || req.Id == 0 || req.AttId == 0 {
			http.Error(w, "id 非法", http.StatusBadRequest)
			return
		}
		var row model.MailTemplateAttachment
		if err := svcCtx.DB.Where("id = ? AND template_id = ?", req.AttId, req.Id).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				http.Error(w, "附件不存在", http.StatusNotFound)
				return
			}
			http.Error(w, "查询失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := svcCtx.DB.Delete(&row).Error; err != nil {
			http.Error(w, "删除失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		abs := filepath.Join(svcCtx.Config.Mail.AttachmentDir, row.FilePath)
		_ = os.Remove(abs)
		httpx.OkJsonCtx(r.Context(), w, map[string]any{"deleted": req.AttId})
	}
}

