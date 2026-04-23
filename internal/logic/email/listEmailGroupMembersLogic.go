package email

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListEmailGroupMembersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListEmailGroupMembersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListEmailGroupMembersLogic {
	return &ListEmailGroupMembersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListEmailGroupMembersLogic) ListEmailGroupMembers(req *types.EmailGroupIdReq) (resp *types.EmailGroupMemberListResp, err error) {
	var rows []struct {
		UserId int64
		Name   string
		Email  string
	}
	if err = l.svcCtx.DB.Raw(`
		SELECT m.user_id AS user_id,
		       IFNULL(u.name,'') AS name,
		       m.email AS email
		FROM email_group_members m
		LEFT JOIN users u ON u.id = m.user_id
		WHERE m.group_id = ?
	`, req.Id).Scan(&rows).Error; err != nil {
		return nil, err
	}
	resp = &types.EmailGroupMemberListResp{List: []types.EmailGroupMember{}}
	for _, r := range rows {
		resp.List = append(resp.List, types.EmailGroupMember{
			UserId: r.UserId,
			Name:   r.Name,
			Email:  r.Email,
		})
	}
	return resp, nil
}
