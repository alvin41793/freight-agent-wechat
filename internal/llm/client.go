package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"freight-agent-wechat/pkg/config"

	"github.com/sashabaranov/go-openai"
)

// Client LLM 客户端
type Client struct {
	client         *openai.Client
	httpClient     *http.Client // 思考模式下用于原生 HTTP 调用
	apiKey         string
	baseURL        string
	model          string
	maxTokens      int
	temperature    float32
	timeout        time.Duration
	thinking       bool // 是否开启思考模式
	thinkingBudget int  // 思考 token 上限（0 = 不限制）
}

// NewClient 创建 LLM 客户端
func NewClient(cfg *config.LLMConfig) (*Client, error) {
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		clientConfig.BaseURL = cfg.BaseURL
	}

	return &Client{
		client:         openai.NewClientWithConfig(clientConfig),
		httpClient:     &http.Client{},
		apiKey:         cfg.APIKey,
		baseURL:        strings.TrimRight(cfg.BaseURL, "/"),
		model:          cfg.Model,
		maxTokens:      cfg.MaxTokens,
		temperature:    float32(cfg.Temperature),
		timeout:        time.Duration(cfg.Timeout) * time.Second,
		thinking:       cfg.Thinking,
		thinkingBudget: cfg.ThinkingBudget,
	}, nil
}

// CompletionRequest 完成请求
type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
	Schema       interface{}
}

// CompletionResponse 完成响应
type CompletionResponse struct {
	Content          string // 模型正文回答（已剪除 think 块）
	ReasoningContent string // 思考过程（思考模式开启时）
	Usage            openai.Usage
}

// Complete 执行对话完成。
// 思考模式开启时绕过 go-openai，使用原生 HTTP 调用以支持 enable_thinking 参数。
func (c *Client) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	if c.thinking {
		return c.completeHTTP(ctx, req)
	}
	return c.completeSDK(ctx, req)
}

// completeSDK 使用 go-openai SDK 发起请求（默认模式）
func (c *Client) completeSDK(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: req.SystemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: req.UserPrompt},
	}
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		MaxTokens:   c.maxTokens,
		Temperature: c.temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no completion choices returned")
	}
	return &CompletionResponse{
		Content: resp.Choices[0].Message.Content,
		Usage:   resp.Usage,
	}, nil
}

// completeHTTP 使用原生 HTTP 发起请求（思考模式专用）
// 直接控制请求体，确保 enable_thinking 正确传达并获取 reasoning_content
func (c *Client) completeHTTP(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type reqBody struct {
		Model          string    `json:"model"`
		Messages       []message `json:"messages"`
		MaxTokens      int       `json:"max_tokens,omitempty"`
		Temperature    float32   `json:"temperature"`
		EnableThinking *bool     `json:"enable_thinking,omitempty"`
		ThinkingBudget int       `json:"thinking_budget,omitempty"` // 0 时忽略（omitempty）
	}
	type respBody struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	enabled := true
	body, err := json.Marshal(reqBody{
		Model: c.model,
		Messages: []message{
			{Role: "system", Content: req.SystemPrompt},
			{Role: "user", Content: req.UserPrompt},
		},
		MaxTokens:      c.maxTokens,
		Temperature:    0, // 思考模式要求 temperature 必须为 0
		EnableThinking: &enabled,
		ThinkingBudget: c.thinkingBudget, // 0 时 omitempty 自动忽略
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}
	defer httpResp.Body.Close()

	respData, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	var resp respBody
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("parse response failed: %w\nbody: %s", err, string(respData))
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("api error: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no completion choices returned, body: %s", string(respData))
	}

	content := resp.Choices[0].Message.Content
	reasoningContent := resp.Choices[0].Message.ReasoningContent
	// 兼容将 <think>...</think> 嵌入 content 的模型
	if reasoningContent == "" {
		content, reasoningContent = extractThinkBlock(content)
	}
	return &CompletionResponse{
		Content:          content,
		ReasoningContent: reasoningContent,
	}, nil
}

// extractThinkBlock 提取并剪除 content 中的 <think>...</think> 块
// 返回：mainContent(剪除思考块后的正文), thinkContent(思考过程)
func extractThinkBlock(content string) (mainContent, thinkContent string) {
	const openTag, closeTag = "<think>", "</think>"
	start := strings.Index(content, openTag)
	end := strings.Index(content, closeTag)
	if start != -1 && end != -1 && end > start {
		thinkContent = strings.TrimSpace(content[start+len(openTag) : end])
		mainContent = strings.TrimSpace(content[end+len(closeTag):])
		return
	}
	return content, ""
}

// CompleteWithJSON 执行 JSON 格式对话完成
// 返回的 string 为思考过程（思考模式开启时），否则为空字符串
func (c *Client) CompleteWithJSON(ctx context.Context, req *CompletionRequest, result interface{}) (string, error) {
	// 在 system prompt 中添加 JSON 格式要求
	systemPrompt := req.SystemPrompt + "\n\n你必须以 JSON 格式输出，不要包含任何其他文本。"

	resp, err := c.Complete(ctx, &CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   req.UserPrompt,
	})
	if err != nil {
		return "", err
	}

	// 清理可能的 markdown 代码块
	content := cleanJSONContent(resp.Content)

	// 解析 JSON
	if err := json.Unmarshal([]byte(content), result); err != nil {
		return resp.ReasoningContent, fmt.Errorf("failed to parse JSON response: %w\ncontent: %s", err, content)
	}

	return resp.ReasoningContent, nil
}

// cleanJSONContent 清理 JSON 内容，移除 markdown 代码块标记
func cleanJSONContent(content string) string {
	// 移除 ```json 和 ``` 标记
	if len(content) > 7 && content[:7] == "```json" {
		content = content[7:]
	}
	if len(content) > 3 && content[:3] == "```" {
		content = content[3:]
	}
	if len(content) > 3 && content[len(content)-3:] == "```" {
		content = content[:len(content)-3]
	}
	return content
}
