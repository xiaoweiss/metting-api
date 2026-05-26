package email

import (
	"context"
	"testing"

	"meeting/internal/config"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/conf"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 集成 smoke test: 需要本地 MySQL 跑 + 已应用 016 migration + hotels 表至少一行 hotel_group 非空。
// go test ./internal/logic/email -run TestPreviewEmailGroupMembers -v
func TestPreviewEmailGroupMembers(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	var c config.Config
	if err := conf.Load("../../../etc/meeting-api.yaml", &c); err != nil {
		t.Skipf("缺少配置文件: %v", err)
	}
	db, err := gorm.Open(mysql.Open(c.DB.DSN), &gorm.Config{})
	if err != nil {
		t.Skipf("DB 连接失败: %v", err)
	}

	svcCtx := &svc.ServiceContext{DB: db, Config: c}

	cases := []struct {
		name      string
		dimension string
		value     string
		wantErr   bool
	}{
		{"dimension 非法 → 400", "role", "any", true},
		{"value 空 → 400", "group", "", true},
		{"按集团命中可能 0 也可能 >0", "group", "万豪集团", false},
		{"按类型命中可能 0 也可能 >0", "type", "Convention", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewPreviewEmailGroupMembersLogic(context.Background(), svcCtx)
			resp, err := l.PreviewEmailGroupMembers(&types.PreviewEmailGroupMembersReq{
				Dimension: tc.dimension,
				Value:     tc.value,
			})
			if tc.wantErr {
				if err == nil {
					t.Errorf("期望返回 error,实际成功: %+v", resp)
				}
				return
			}
			if err != nil {
				t.Errorf("期望成功,返回 error: %v", err)
				return
			}
			if resp.Count != len(resp.List) {
				t.Errorf("count(%d) 应 = len(list)(%d)", resp.Count, len(resp.List))
			}
			for _, item := range resp.List {
				if item.Email == "" {
					t.Errorf("item 不应该有空 email: %+v", item)
				}
			}
		})
	}
}
