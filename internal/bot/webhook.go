package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Bot 企业微信机器人
type Bot struct {
	corpID         string
	corpSecret     string
	agentID        int
	token          string
	encodingAESKey string
	accessToken    string
	tokenExpiresAt time.Time
	httpClient     *http.Client
}

// NewBot 创建企业微信机器人
func NewBot(corpID, corpSecret string, agentID int, token, encodingAESKey string) *Bot {
	return &Bot{
		corpID:         corpID,
		corpSecret:     corpSecret,
		agentID:        agentID,
		token:          token,
		encodingAESKey: encodingAESKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Message 企业微信消息
type Message struct {
	ToUser  string        `json:"touser"`
	ToParty string        `json:"toparty"`
	ToTag   string        `json:"totag"`
	MsgType string        `json:"msgtype"`
	AgentID int           `json:"agentid"`
	Text    *TextMessage  `json:"text,omitempty"`
	Image   *ImageMessage `json:"image,omitempty"`
	File    *FileMessage  `json:"file,omitempty"`
}

// TextMessage 文本消息
type TextMessage struct {
	Content string `json:"content"`
}

// ImageMessage 图片消息
type ImageMessage struct {
	MediaID string `json:"media_id"`
}

// FileMessage 文件消息
type FileMessage struct {
	MediaID string `json:"media_id"`
}

// WebhookRequest 智能机器人回调请求（企业微信智能机器人格式）
type WebhookRequest struct {
	MsgID       string     `json:"msgid"`
	AIBotID     string     `json:"aibotid"`
	ChatID      string     `json:"chatid"`
	ChatType    string     `json:"chattype"` // group / single
	From        FromUser   `json:"from"`
	ResponseURL string     `json:"response_url"` // 回复用的临时URL，核心字段
	MsgType     string     `json:"msgtype"`
	Text        *TextBody  `json:"text,omitempty"`
	Image       *MediaBody `json:"image,omitempty"`
	File        *MediaBody `json:"file,omitempty"`
}

// FromUser 消息发送者
type FromUser struct {
	UserID string `json:"userid"`
}

// TextBody 文本消息体
type TextBody struct {
	Content string `json:"content"`
}

// MediaBody 媒体消息体
type MediaBody struct {
	MediaID string `json:"media_id"`
}

// GetContent 获取消息文本内容（自动剥离@机器人前缀）
func (r *WebhookRequest) GetContent() string {
	if r.Text == nil {
		return ""
	}
	content := strings.TrimSpace(r.Text.Content)
	// 剥离 @机器人名称 前缀
	if idx := strings.Index(content, " "); idx != -1 && strings.HasPrefix(content, "@") {
		content = strings.TrimSpace(content[idx+1:])
	}
	return content
}

// WebhookResponse 智能机器人回复格式
type WebhookResponse struct {
	MsgType string        `json:"msgtype"`
	Text    *TextMessage  `json:"text,omitempty"`
	Image   *ImageMessage `json:"image,omitempty"`
	File    *FileMessage  `json:"file,omitempty"`
}

// ReplyText 通过 response_url 回复文本（智能机器人专用）
func (b *Bot) ReplyText(responseURL, content string) error {
	payload := WebhookResponse{
		MsgType: "text",
		Text:    &TextMessage{Content: content},
	}
	return b.replyViaResponseURL(responseURL, payload)
}

// ReplyImage 通过 response_url 回复图片（智能机器人专用）
func (b *Bot) ReplyImage(responseURL, mediaID string) error {
	payload := WebhookResponse{
		MsgType: "image",
		Image:   &ImageMessage{MediaID: mediaID},
	}
	return b.replyViaResponseURL(responseURL, payload)
}

// ReplyFile 通过 response_url 回复文件（智能机器人专用）
func (b *Bot) ReplyFile(responseURL, mediaID string) error {
	payload := WebhookResponse{
		MsgType: "file",
		File:    &FileMessage{MediaID: mediaID},
	}
	return b.replyViaResponseURL(responseURL, payload)
}

// replyViaResponseURL 向 response_url 发送回复
func (b *Bot) replyViaResponseURL(responseURL string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal reply: %w", err)
	}

	req, err := http.NewRequest("POST", responseURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create reply request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send reply: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode reply response: %w", err)
	}
	if result.Errcode != 0 {
		return fmt.Errorf("reply error: %d - %s", result.Errcode, result.Errmsg)
	}
	return nil
}

// GetAccessToken 获取 access token
func (b *Bot) GetAccessToken() (string, error) {
	// 如果 token 未过期，直接返回
	if b.accessToken != "" && time.Now().Before(b.tokenExpiresAt) {
		return b.accessToken, nil
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s",
		b.corpID, b.corpSecret)

	resp, err := b.httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode     int    `json:"errcode"`
		Errmsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Errcode != 0 {
		return "", fmt.Errorf("wechat api error: %d - %s", result.Errcode, result.Errmsg)
	}

	b.accessToken = result.AccessToken
	b.tokenExpiresAt = time.Now().Add(time.Duration(result.ExpiresIn-300) * time.Second)

	return b.accessToken, nil
}

// SendText 发送文本消息
func (b *Bot) SendText(userID, content string) error {
	token, err := b.GetAccessToken()
	if err != nil {
		return err
	}

	msg := Message{
		ToUser:  userID,
		MsgType: "text",
		AgentID: b.agentID,
		Text: &TextMessage{
			Content: content,
		},
	}

	return b.sendMessage(token, msg)
}

// SendGroupText 发送群文本消息
func (b *Bot) SendGroupText(chatID, content string) error {
	token, err := b.GetAccessToken()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/appchat/send?access_token=%s", token)

	msg := map[string]interface{}{
		"chatid":  chatID,
		"msgtype": "text",
		"text": map[string]string{
			"content": content,
		},
	}

	return b.sendRequest(url, msg)
}

// SendImage 发送图片消息
func (b *Bot) SendImage(userID, mediaID string) error {
	token, err := b.GetAccessToken()
	if err != nil {
		return err
	}

	msg := Message{
		ToUser:  userID,
		MsgType: "image",
		AgentID: b.agentID,
		Image: &ImageMessage{
			MediaID: mediaID,
		},
	}

	return b.sendMessage(token, msg)
}

// SendGroupImage 发送群图片消息
func (b *Bot) SendGroupImage(chatID, mediaID string) error {
	token, err := b.GetAccessToken()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/appchat/send?access_token=%s", token)

	msg := map[string]interface{}{
		"chatid":  chatID,
		"msgtype": "image",
		"image": map[string]string{
			"media_id": mediaID,
		},
	}

	return b.sendRequest(url, msg)
}

// UploadMedia 上传媒体文件
func (b *Bot) UploadMedia(mediaType string, filename string, data []byte) (string, error) {
	token, err := b.GetAccessToken()
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/media/upload?access_token=%s&type=%s",
		token, mediaType)

	// 创建 multipart form
	body := &bytes.Buffer{}
	writer := createMultipartWriter(body, filename, data)

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload media: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
		MediaID string `json:"media_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Errcode != 0 {
		return "", fmt.Errorf("wechat api error: %d - %s", result.Errcode, result.Errmsg)
	}

	return result.MediaID, nil
}

// sendMessage 发送消息
func (b *Bot) sendMessage(token string, msg Message) error {
	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=%s", token)
	return b.sendRequest(url, msg)
}

// sendRequest 发送请求
func (b *Bot) sendRequest(url string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Errcode != 0 {
		return fmt.Errorf("wechat api error: %d - %s", result.Errcode, result.Errmsg)
	}

	return nil
}

// createMultipartWriter 创建 multipart writer
func createMultipartWriter(w io.Writer, _ string, _ []byte) *multipartWriter {
	// 简化实现,实际项目中应使用 mime/multipart
	return &multipartWriter{w: w}
}

// multipartWriter 简化的 multipart writer
type multipartWriter struct {
	w io.Writer
}

// FormDataContentType 返回 Content-Type
func (w *multipartWriter) FormDataContentType() string {
	return "multipart/form-data"
}
