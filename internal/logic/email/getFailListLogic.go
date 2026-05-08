package email

import (
	"context"
	"encoding/json"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFailListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetFailListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFailListLogic {
	return &GetFailListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// 兼容历史数据：fail_list 可能是 ["a@x.com","b@y.com"] 这种纯字符串数组，
// 也可能是新版的 [{email, error}]。两种格式都解析。
func (l *GetFailListLogic) GetFailList(req *types.EmailLogIdReq) (*types.FailListResp, error) {
	var lg model.EmailLog
	if err := l.svcCtx.DB.First(&lg, req.Id).Error; err != nil {
		return nil, err
	}
	resp := &types.FailListResp{List: []types.FailListItem{}}
	if lg.FailList == "" || lg.FailList == "null" {
		return resp, nil
	}
	// 先试 [{email, error}]
	var items []types.FailListItem
	if err := json.Unmarshal([]byte(lg.FailList), &items); err == nil {
		resp.List = items
		return resp, nil
	}
	// fallback 试 ["a@x.com","b@y.com"]
	var emails []string
	if err := json.Unmarshal([]byte(lg.FailList), &emails); err == nil {
		for _, e := range emails {
			resp.List = append(resp.List, types.FailListItem{Email: e})
		}
	}
	return resp, nil
}
