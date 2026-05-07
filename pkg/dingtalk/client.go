package dingtalk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	AppKey    string
	AppSecret string
}

type UserInfo struct {
	UnionId string
	UserId  string
	Name    string
	Email   string
}

// GetAccessToken 获取企业内部应用 access_token
func (c *Client) GetAccessToken() (string, error) {
	url := "https://oapi.dingtalk.com/gettoken?appkey=" + c.AppKey + "&appsecret=" + c.AppSecret
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &result)
	if result.ErrCode != 0 {
		return "", fmt.Errorf("获取 access_token 失败: %s", result.ErrMsg)
	}
	return result.AccessToken, nil
}

// GetUserByCode 用免登 code 换取用户信息（企业内部应用：corp access_token + topapi/v2/user/getuserinfo）
func (c *Client) GetUserByCode(code string) (*UserInfo, error) {
	accessToken, err := c.GetAccessToken()
	if err != nil {
		return nil, fmt.Errorf("获取 corp access_token 失败: %w", err)
	}

	// Step1: code 换 userid
	url1 := "https://oapi.dingtalk.com/topapi/v2/user/getuserinfo?access_token=" + accessToken
	body1 := fmt.Sprintf(`{"code":"%s"}`, code)
	req1, err := http.NewRequest("POST", url1, strings.NewReader(body1))
	if err != nil {
		return nil, err
	}
	req1.Header.Set("Content-Type", "application/json")

	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		return nil, fmt.Errorf("请求 getuserinfo 失败: %w", err)
	}
	defer resp1.Body.Close()
	respBody1, _ := io.ReadAll(resp1.Body)

	var ui struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Result  struct {
			UserId string `json:"userid"`
			Name   string `json:"name"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody1, &ui); err != nil {
		return nil, fmt.Errorf("解析 getuserinfo 响应失败: %w", err)
	}
	if ui.ErrCode != 0 || ui.Result.UserId == "" {
		return nil, fmt.Errorf("getuserinfo 失败: %s", string(respBody1))
	}

	// Step2: userid 换 unionid + email
	url2 := "https://oapi.dingtalk.com/topapi/v2/user/get?access_token=" + accessToken
	body2 := fmt.Sprintf(`{"userid":"%s"}`, ui.Result.UserId)
	req2, err := http.NewRequest("POST", url2, strings.NewReader(body2))
	if err != nil {
		return nil, err
	}
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("请求 user/get 失败: %w", err)
	}
	defer resp2.Body.Close()
	respBody2, _ := io.ReadAll(resp2.Body)

	var ud struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Result  struct {
			UserId  string `json:"userid"`
			UnionId string `json:"unionid"`
			Name    string `json:"name"`
			Email   string `json:"email"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody2, &ud); err != nil {
		return nil, fmt.Errorf("解析 user/get 响应失败: %w", err)
	}
	if ud.ErrCode != 0 || ud.Result.UnionId == "" {
		return nil, fmt.Errorf("user/get 失败: %s", string(respBody2))
	}

	return &UserInfo{
		UnionId: ud.Result.UnionId,
		UserId:  ud.Result.UserId,
		Name:    ud.Result.Name,
		Email:   ud.Result.Email,
	}, nil
}

// GetCorpToken 获取企业应用新版 token（用于多维表格 API）
func (c *Client) GetCorpToken() (string, error) {
	url := "https://api.dingtalk.com/v1.0/oauth2/accessToken"
	body := fmt.Sprintf(`{"appKey":"%s","appSecret":"%s"}`, c.AppKey, c.AppSecret)

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int    `json:"expireIn"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	json.Unmarshal(respBody, &result)
	if result.AccessToken == "" {
		return "", fmt.Errorf("获取新版 token 失败: %s", string(respBody))
	}
	return result.AccessToken, nil
}
