package email

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RetryEmailLogLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRetryEmailLogLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RetryEmailLogLogic {
	return &RetryEmailLogLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// RetryEmailLog 同步触发：把指定日志里的失败邮箱重发一遍。
// 内部已经 goroutine 并发，handler 在收件人不多时直接等结果即可；
// 若收件人多想立刻返回，前端调用 retry-all 走异步路径。
func (l *RetryEmailLogLogic) RetryEmailLog(req *types.EmailLogIdReq) (*types.BaseResp, error) {
	if _, err := l.svcCtx.BlastEngine.RetryFailed(l.ctx, req.Id); err != nil {
		return nil, err
	}
	return &types.BaseResp{Message: "ok"}, nil
}
