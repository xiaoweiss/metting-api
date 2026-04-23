package auth

import (
	"net/http"
	"time"

	"meeting/internal/model"
	"meeting/internal/svc"

	"github.com/golang-jwt/jwt/v4"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func DevTokenHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var user model.User
		if err := svcCtx.DB.Where("status = ?", "active").First(&user).Error; err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		var hotelIds []int64
		svcCtx.DB.Raw("SELECT hotel_id FROM user_hotel_perms WHERE user_id = ?", user.Id).Scan(&hotelIds)

		claims := jwt.MapClaims{
			"userId":  user.Id,
			"isAdmin": user.IsAdmin,
			"exp":     time.Now().Add(24 * time.Hour).Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(svcCtx.Config.JWT.Secret))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{
			"token":    tokenStr,
			"name":     user.Name,
			"status":   user.Status,
			"hotelIds": hotelIds,
		})
	}
}
