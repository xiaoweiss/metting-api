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

// GetUserByCode 用免登 code 换取用户信息（新版 API）
func (c *Client) GetUserByCode(code string) (*UserInfo, error) {
	// Step1: code 换 userAccessToken
	tokenURL := "https://api.dingtalk.com/v1.0/oauth2/userAccessToken"
	tokenBody := fmt.Sprintf(`{"clientId":"%s","clientSecret":"%s","code":"%s","grantType":"authorization_code"}`,
		c.AppKey, c.AppSecret, code)

	tokenReq, err := http.NewRequest("POST", tokenURL, strings.NewReader(tokenBody))
	if err != nil {
		return nil, err
	}
	tokenReq.Header.Set("Content-Type", "application/json")

	tokenResp, err := http.DefaultClient.Do(tokenReq)
	if err != nil {
		return nil, fmt.Errorf("请求 userAccessToken 失败: %w", err)
	}
	defer tokenResp.Body.Close()

	tokenRespBody, _ := io.ReadAll(tokenResp.Body)

	var tokenResult struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpireIn     int    `json:"expireIn"`
	}
	if err := json.Unmarshal(tokenRespBody, &tokenResult); err != nil {
		return nil, fmt.Errorf("解析 userAccessToken 响应失败: %w", err)
	}
	if tokenResult.AccessToken == "" {
		return nil, fmt.Errorf("获取 userAccessToken 失败: %s", string(tokenRespBody))
	}

	// Step2: 用 userAccessToken 获取用户信息
	meURL := "https://api.dingtalk.com/v1.0/contact/users/me"
	meReq, err := http.NewRequest("GET", meURL, nil)
	if err != nil {
		return nil, err
	}
	meReq.Header.Set("x-acs-dingtalk-access-token", tokenResult.AccessToken)

	meResp, err := http.DefaultClient.Do(meReq)
	if err != nil {
		return nil, fmt.Errorf("请求用户信息失败: %w", err)
	}
	defer meResp.Body.Close()

	meRespBody, _ := io.ReadAll(meResp.Body)

	var userResult struct {
		Nick    string `json:"nick"`
		UnionId string `json:"unionId"`
		OpenId  string `json:"openId"`
		Email   string `json:"email"`
		Mobile  string `json:"mobile"`
	}
	if err := json.Unmarshal(meRespBody, &userResult); err != nil {
		return nil, fmt.Errorf("解析用户信息失败: %w", err)
	}
	if userResult.UnionId == "" {
		return nil, fmt.Errorf("获取用户信息失败: %s", string(meRespBody))
	}

	return &UserInfo{
		UnionId: userResult.UnionId,
		UserId:  userResult.OpenId,
		Name:    userResult.Nick,
		Email:   userResult.Email,
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
