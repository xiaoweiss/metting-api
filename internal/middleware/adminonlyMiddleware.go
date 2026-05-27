package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"meeting/pkg/audit"

	"gorm.io/gorm"
)

// clientIp 取 X-Forwarded-For 第一段 / X-Real-IP / RemoteAddr 兜底。
func clientIp(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	// RemoteAddr 形如 "1.2.3.4:5678",截掉端口
	if i := strings.LastIndexByte(r.RemoteAddr, ':'); i > 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}

func NewAdminOnlyMiddleware(db *gorm.DB) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			userIdStr := r.Header.Get("X-User-Id")
			userId, err := strconv.ParseInt(userIdStr, 10, 64)
			if err != nil || userId == 0 {
				forbidden(w, "无权限")
				return
			}

			var row struct {
				Name     string
				IsAdmin  bool
				Status   string
				RoleId   *int64
				RoleName *string
				Apis     *string
			}
			db.Raw(`
				SELECT u.name, u.is_admin, u.status, u.role_id,
					   r.name AS role_name, r.apis
				FROM users u
				LEFT JOIN roles r ON r.id = u.role_id
				WHERE u.id = ? AND u.status = 'active'
			`, userId).Scan(&row)

			// audit: 注入操作人到 ctx,下游 logic 通过 audit.Log 隐式拿到
			r = r.WithContext(audit.WithActor(r.Context(), userId, row.Name, clientIp(r)))

			if row.Status != "active" {
				forbidden(w, "账号未激活")
				return
			}

			if !row.IsAdmin && row.RoleId == nil {
				forbidden(w, "需要管理员权限")
				return
			}

			// super_admin bypasses API check
			if row.RoleName != nil && *row.RoleName == "super_admin" {
				next(w, r)
				return
			}

			// For users with is_admin=true but no role, allow all (backward compat)
			if row.IsAdmin && row.RoleId == nil {
				next(w, r)
				return
			}

			// Check API permission via role's apis JSON
			if row.Apis != nil {
				var allowed []string
				if err := json.Unmarshal([]byte(*row.Apis), &allowed); err == nil {
					reqPath := r.URL.Path
					for _, pattern := range allowed {
						if matchPath(pattern, reqPath) {
							next(w, r)
							return
						}
					}
				}
			}

			forbidden(w, "无此接口权限")
		}
	}
}

func forbidden(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(`{"code":403,"message":"` + msg + `"}`))
}

func matchPath(pattern, path string) bool {
	if pattern == path {
		return true
	}
	// /api/admin/* matches /api/admin/anything/nested
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}
	return false
}
