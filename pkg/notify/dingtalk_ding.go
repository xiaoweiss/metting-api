package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"meeting/internal/config"
	"meeting/pkg/dingtalk"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

// DingTalkDingConfig 钉钉工作通知配置（ding 消息走企业自建应用的 workMessage asyncsend_v2）
type DingTalkDingConfig struct {
	AgentId     int64 `json:"agentId"`     // 覆盖 yaml 里的 AgentId
	UseAppToken bool  `json:"useAppToken"` // 用应用 token 发（true）还是企业 token
}

type DingTalkDingSender struct {
	DB     *gorm.DB
	Cfg    config.Config
	Client *dingtalk.Client
}

func (s *DingTalkDingSender) Channel() Channel { return ChannelDingTalkDing }

// Send 通过钉钉工作通知接口 asyncsend_v2 发消息
// 文档：https://open.dingtalk.com/document/orgapp/asynchronous-sending-of-enterprise-session-messages
//   - 接受 msg.DingUserIds 或 msg.Unionids（会先转 userId）
//   - markdown/text 都支持，优先 markdown
//   - asyncsend_v2 是异步的，拿到 task_id 后立即轮询 getsendresult 一次，把
//     invalid / forbidden 列表反馈给调用方，避免"报成功但没收到"的黑洞
func (s *DingTalkDingSender) Send(ctx context.Context, msg Message) error {
	var cfg DingTalkDingConfig
	enabled, err := LoadConfig(s.DB, ChannelDingTalkDing, &cfg)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("钉钉 Ding 通知未启用")
	}

	agentId := cfg.AgentId
	if agentId == 0 {
		agentId = s.Cfg.DingTalk.AgentId
	}
	if agentId == 0 {
		return errors.New("AgentId 未配置")
	}

	token, err := s.Client.GetAccessToken()
	if err != nil {
		return err
	}

	// 收集 userIds（unionId → userId）
	userIds := append([]string{}, msg.DingUserIds...)
	var convertErrs []string
	for _, uid := range msg.Unionids {
		userId, err := unionIdToUserId(token, uid)
		if err != nil {
			convertErrs = append(convertErrs, fmt.Sprintf("%s: %v", uid, err))
			continue
		}
		if userId == "" {
			convertErrs = append(convertErrs, fmt.Sprintf("%s: 返回 userid 为空（可能不是本企业员工）", uid))
			continue
		}
		userIds = append(userIds, userId)
	}
	if len(userIds) == 0 {
		base := "钉钉 Ding 收件人为空（没有可用的 userId / unionId）"
		if len(convertErrs) > 0 {
			return fmt.Errorf("%s; unionId 解析明细: %s", base, strings.Join(convertErrs, " | "))
		}
		return errors.New(base)
	}

	// 组 payload
	content := firstNonEmpty(msg.Markdown, msg.Text)
	var ddMsg map[string]interface{}
	if msg.Markdown != "" {
		ddMsg = map[string]interface{}{
			"msgtype":  "markdown",
			"markdown": map[string]string{"title": firstNonEmpty(msg.Title, "通知"), "text": content},
		}
	} else {
		ddMsg = map[string]interface{}{
			"msgtype": "text",
			"text":    map[string]string{"content": content},
		}
	}

	payload := map[string]interface{}{
		"agent_id":    agentId,
		"userid_list": strings.Join(userIds, ","),
		"msg":         ddMsg,
	}
	body, _ := json.Marshal(payload)

	sendURL := "https://oapi.dingtalk.com/topapi/message/corpconversation/asyncsend_v2?access_token=" + token
	req, _ := http.NewRequestWithContext(ctx, "POST", sendURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)

	var sendResp struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
		TaskId  int64  `json:"task_id"`
	}
	_ = json.Unmarshal(rb, &sendResp)
	if sendResp.Errcode != 0 {
		return fmt.Errorf("钉钉 Ding 发送失败[%d]: %s", sendResp.Errcode, sendResp.Errmsg)
	}

	logx.Infof("[DingTalkDing] asyncsend_v2 enqueued task_id=%d agent_id=%d userid_list=%s",
		sendResp.TaskId, agentId, strings.Join(userIds, ","))

	// 轮询任务结果：asyncsend_v2 是异步，需要等一下才能查到结果。
	// 最多等 ~2 秒，3 次尝试。
	sr, queryErr := pollSendResult(ctx, token, agentId, sendResp.TaskId)
	if queryErr != nil {
		logx.Errorf("[DingTalkDing] getsendresult task_id=%d 查询失败: %v（但发送本身已提交）", sendResp.TaskId, queryErr)
		return nil // 不阻塞调用方，发送本身已经提交
	}

	logx.Infof("[DingTalkDing] task_id=%d send_result=%+v", sendResp.TaskId, sr)

	// 有 invalid / forbidden → 这次发送实际上是失败的，抛错让前端看得见
	var problems []string
	if len(sr.InvalidUserIdList) > 0 {
		problems = append(problems, fmt.Sprintf("invalid=%s（非本企业员工或 userid 错）", strings.Join(sr.InvalidUserIdList, ",")))
	}
	if len(sr.ForbiddenUserIdList) > 0 {
		problems = append(problems, fmt.Sprintf("forbidden=%s（钉钉拒收；通常是：应用未勾选「工作通知」权限，或接收人不在应用可见范围内）", strings.Join(sr.ForbiddenUserIdList, ",")))
	}
	if len(sr.FailedUserIdList) > 0 {
		problems = append(problems, fmt.Sprintf("failed=%s", strings.Join(sr.FailedUserIdList, ",")))
	}
	if len(problems) > 0 {
		return fmt.Errorf("钉钉工作通知提交成功但钉钉投递失败 [task_id=%d]: %s",
			sendResp.TaskId, strings.Join(problems, "; "))
	}
	return nil
}

// sendResult getsendresult 返回的 send_result 部分
type sendResult struct {
	InvalidUserIdList   []string `json:"invalid_user_id_list"`
	ForbiddenUserIdList []string `json:"forbidden_user_id_list"`
	FailedUserIdList    []string `json:"failed_user_id_list"`
	ReadUserIdList      []string `json:"read_user_id_list"`
	UnreadUserIdList    []string `json:"unread_user_id_list"`
}

// pollSendResult 轮询任务结果。钉钉是异步投递，task_id 刚拿到时查通常是空，
// 所以重试几次。最多等 ~1.3 秒，给上层 handler 留出充足的 timeout 预算。
func pollSendResult(ctx context.Context, accessToken string, agentId, taskId int64) (*sendResult, error) {
	var lastErr error
	waits := []time.Duration{200 * time.Millisecond, 400 * time.Millisecond, 700 * time.Millisecond}
	for _, w := range waits {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(w):
		}
		sr, err := getSendResult(ctx, accessToken, agentId, taskId)
		if err != nil {
			lastErr = err
			continue
		}
		// 只要任何一个列表非空，说明任务已经有结果（或已经 read/unread/invalid...）
		if len(sr.InvalidUserIdList)+len(sr.ForbiddenUserIdList)+len(sr.FailedUserIdList)+
			len(sr.ReadUserIdList)+len(sr.UnreadUserIdList) > 0 {
			return sr, nil
		}
		// 空列表继续等
	}
	if lastErr != nil {
		return nil, lastErr
	}
	// 超时但没报错 → 任务仍在排队，当作正常返回（读未读都空）
	return &sendResult{}, nil
}

func getSendResult(ctx context.Context, accessToken string, agentId, taskId int64) (*sendResult, error) {
	body, _ := json.Marshal(map[string]int64{"agent_id": agentId, "task_id": taskId})
	u := "https://oapi.dingtalk.com/topapi/message/corpconversation/getsendresult?access_token=" + accessToken
	req, _ := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)

	var r struct {
		Errcode    int        `json:"errcode"`
		Errmsg     string     `json:"errmsg"`
		SendResult sendResult `json:"send_result"`
	}
	if err := json.Unmarshal(rb, &r); err != nil {
		return nil, err
	}
	if r.Errcode != 0 {
		return nil, fmt.Errorf("getsendresult[%d]: %s", r.Errcode, r.Errmsg)
	}
	return &r.SendResult, nil
}

// unionIdToUserId 企业内部应用接口：unionid → userid
func unionIdToUserId(accessToken, unionId string) (string, error) {
	body, _ := json.Marshal(map[string]string{"unionid": unionId})
	url := "https://oapi.dingtalk.com/topapi/user/getbyunionid?access_token=" + accessToken
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)

	var r struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
		Result  struct {
			Userid string `json:"userid"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rb, &r); err != nil {
		return "", err
	}
	if r.Errcode != 0 {
		return "", fmt.Errorf("getbyunionid 失败[%d]: %s", r.Errcode, r.Errmsg)
	}
	return r.Result.Userid, nil
}
