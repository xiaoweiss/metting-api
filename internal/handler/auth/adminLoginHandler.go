package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"meeting/internal/model"
	"meeting/internal/svc"

	"github.com/golang-jwt/jwt/v4"
	"github.com/zeromicro/go-zero/rest/httpx"
	"golang.org/x/crypto/bcrypt"
)

type adminLoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func AdminLoginHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req adminLoginReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		var user model.User
		if err := svcCtx.DB.Where("name = ? AND is_admin = ?", req.Username, true).First(&user).Error; err != nil {
			http.Error(w, "用户名或密码错误", http.StatusUnauthorized)
			return
		}

		if user.AdminPassword == "" {
			if req.Password != svcCtx.Config.JWT.Secret {
				http.Error(w, "用户名或密码错误", http.StatusUnauthorized)
				return
			}
		} else {
			if err := bcrypt.CompareHashAndPassword([]byte(user.AdminPassword), []byte(req.Password)); err != nil {
				http.Error(w, "用户名或密码错误", http.StatusUnauthorized)
				return
			}
		}

		claims := jwt.MapClaims{
			"userId":  user.Id,
			"isAdmin": true,
			"exp":     time.Now().Add(24 * time.Hour).Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, _ := token.SignedString([]byte(svcCtx.Config.JWT.Secret))

		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{
			"token": tokenStr,
		})
	}
}
