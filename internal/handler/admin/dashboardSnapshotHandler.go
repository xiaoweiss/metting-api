package admin

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/rest/httpx"
	"gorm.io/gorm/clause"

	"meeting/internal/model"
	"meeting/internal/svc"
)

const (
	maxSnapshotBytes = 5 << 20 // 5MB
	parseFormBytes   = 6 << 20 // 6MB,留 1MB buffer 给 multipart 头
)

func validMode(m string) bool   { return m == "occupancy" || m == "bookings" }
func validFormat(f string) bool { return f == "png" || f == "pdf" }

// shanghaiDate 把 YYYY-MM-DD 解析为 Asia/Shanghai 0:00
func shanghaiDate(s string) (time.Time, error) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	return time.ParseInLocation("2006-01-02", s, loc)
}

// UploadDashboardSnapshotHandler POST /api/admin/dashboard-snapshots
//
// 接收 multipart/form-data: file + hotelId + date(YYYY-MM-DD) + mode + format,
// 写入磁盘并 UPSERT (hotel_id, snapshot_date, mode, format) 一行,旧文件路径不同则删旧物理文件。
func UploadDashboardSnapshotHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(parseFormBytes); err != nil {
			http.Error(w, "解析表单失败: "+err.Error(), http.StatusBadRequest)
			return
		}

		hotelId, err := strconv.ParseInt(r.FormValue("hotelId"), 10, 64)
		if err != nil || hotelId == 0 {
			http.Error(w, "hotelId 非法", http.StatusBadRequest)
			return
		}
		dateStr := strings.TrimSpace(r.FormValue("date"))
		snapDate, err := shanghaiDate(dateStr)
		if err != nil {
			http.Error(w, "date 必须为 YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		mode := strings.TrimSpace(r.FormValue("mode"))
		if !validMode(mode) {
			http.Error(w, "mode 必须为 occupancy 或 bookings", http.StatusBadRequest)
			return
		}
		format := strings.TrimSpace(r.FormValue("format"))
		if !validFormat(format) {
			http.Error(w, "format 必须为 png 或 pdf", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "缺少 file 字段", http.StatusBadRequest)
			return
		}
		defer file.Close()
		if header.Size > maxSnapshotBytes {
			http.Error(w, "文件超过 5MB 上限", http.StatusRequestEntityTooLarge)
			return
		}

		uploadedBy := (*int64)(nil)
		if uid, err := strconv.ParseInt(r.Header.Get("X-User-Id"), 10, 64); err == nil && uid > 0 {
			uploadedBy = &uid
		}

		// 相对路径: {YYYY}/{MM}/dashboard-{hotelId}-{YYYY-MM-DD}-{mode}.{format}
		ymd := snapDate.Format("2006-01-02")
		baseName := fmt.Sprintf("dashboard-%d-%s-%s.%s", hotelId, ymd, mode, format)
		relPath := filepath.Join(snapDate.Format("2006"), snapDate.Format("01"), baseName)
		absPath := filepath.Join(svcCtx.Config.Mail.SnapshotDir, relPath)

		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			http.Error(w, "创建目录失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// 临时文件 + rename 保证原子性
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

		// 查现有行,若 file_path 不同则删旧物理文件
		var old model.DashboardSnapshot
		svcCtx.DB.
			Where("hotel_id = ? AND snapshot_date = ? AND mode = ? AND format = ?",
				hotelId, snapDate, mode, format).
			First(&old)
		if old.Id > 0 && old.FilePath != relPath {
			oldAbs := filepath.Join(svcCtx.Config.Mail.SnapshotDir, old.FilePath)
			_ = os.Remove(oldAbs)
		}

		row := model.DashboardSnapshot{
			HotelId:      hotelId,
			SnapshotDate: snapDate,
			Mode:         mode,
			Format:       format,
			FilePath:     relPath,
			FileSize:     written,
			UploadedBy:   uploadedBy,
		}
		// UPSERT
		if err := svcCtx.DB.
			Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "hotel_id"}, {Name: "snapshot_date"}, {Name: "mode"}, {Name: "format"},
				},
				DoUpdates: clause.AssignmentColumns([]string{"file_path", "file_size", "uploaded_by", "uploaded_at"}),
			}).
			Create(&row).Error; err != nil {
			os.Remove(absPath)
			http.Error(w, "DB 写入失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		httpx.OkJsonCtx(r.Context(), w, map[string]any{
			"id":         row.Id,
			"filePath":   relPath,
			"uploadedAt": time.Now().Format(time.RFC3339),
		})
	}
}

// ListDashboardSnapshotsHandler GET /api/admin/dashboard-snapshots?hotelId=&date=&mode=&format=
func ListDashboardSnapshotsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := svcCtx.DB.Model(&model.DashboardSnapshot{})
		if v := r.URL.Query().Get("hotelId"); v != "" {
			if id, err := strconv.ParseInt(v, 10, 64); err == nil {
				q = q.Where("hotel_id = ?", id)
			}
		}
		if v := r.URL.Query().Get("date"); v != "" {
			if d, err := shanghaiDate(v); err == nil {
				q = q.Where("snapshot_date = ?", d)
			}
		}
		if v := r.URL.Query().Get("mode"); v != "" && validMode(v) {
			q = q.Where("mode = ?", v)
		}
		if v := r.URL.Query().Get("format"); v != "" && validFormat(v) {
			q = q.Where("format = ?", v)
		}
		var rows []model.DashboardSnapshot
		q.Order("uploaded_at DESC").Limit(200).Find(&rows)
		httpx.OkJsonCtx(r.Context(), w, map[string]any{"list": rows})
	}
}

// ServeDashboardSnapshotHandler GET /admin/snapshots/:year/:month/:filename
// 提供静态预览。路径校验防止穿越。
func ServeDashboardSnapshotHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Year     string `path:"year"`
			Month    string `path:"month"`
			Filename string `path:"filename"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			http.Error(w, "非法路径: "+err.Error(), http.StatusBadRequest)
			return
		}
		// 严格校验:year=4 位数字, month=2 位数字, filename 不含 / .. 等
		if !isYear(req.Year) || !isMonth(req.Month) || !isSafeFilename(req.Filename) {
			http.Error(w, "非法路径", http.StatusBadRequest)
			return
		}
		relPath := filepath.Join(req.Year, req.Month, req.Filename)
		absPath := filepath.Join(svcCtx.Config.Mail.SnapshotDir, relPath)
		// 再校验 absPath 落在 SnapshotDir 内
		root, _ := filepath.Abs(svcCtx.Config.Mail.SnapshotDir)
		got, _ := filepath.Abs(absPath)
		if !strings.HasPrefix(got, root+string(os.PathSeparator)) {
			http.Error(w, "非法路径", http.StatusBadRequest)
			return
		}
		f, err := os.Open(absPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "读取失败", http.StatusInternalServerError)
			return
		}
		defer f.Close()
		stat, _ := f.Stat()
		http.ServeContent(w, r, req.Filename, stat.ModTime(), f)
	}
}

func isYear(s string) bool {
	return len(s) == 4 && allDigits(s)
}
func isMonth(s string) bool {
	return len(s) == 2 && allDigits(s)
}
func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func isSafeFilename(s string) bool {
	if s == "" || strings.Contains(s, "..") || strings.Contains(s, "/") || strings.Contains(s, "\\") {
		return false
	}
	return true
}
