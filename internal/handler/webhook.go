package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"freight-agent-wechat/internal/llm"
)

// WebhookHandler 插件处理器
type WebhookHandler struct {
	llmService *llm.Service
	httpClient *http.Client
}

// NewWebhookHandler 创建处理器
func NewWebhookHandler(llmService *llm.Service) *WebhookHandler {
	return &WebhookHandler{
		llmService: llmService,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// PluginRequest 企业微信智能机器人插件请求格式
type PluginRequest struct {
	Query       string `json:"query"`
	UserID      string `json:"userid"`
	ChatID      string `json:"chatid"`
	ChatType    string `json:"chattype"`
	ResponseURL string `json:"response_url"` // 异步回调地址
}

// PluginResponse 插件返回格式
type PluginResponse struct {
	Content string `json:"content"`
}

// StreamResponse 流式响应格式
type StreamResponse struct {
	Content string `json:"content"`
	Stream  bool   `json:"stream"`
	Finish  bool   `json:"finish"`
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

// Handle 处理插件请求
func (h *WebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var req PluginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Failed to decode plugin request: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		writeJSON(w, PluginResponse{Content: "请提供运价信息，例如：MSK 美东 SAV/NWK 2500/40HQ"})
		return
	}

	log.Printf("[plugin] query: %s, response_url: %s", req.Query, req.ResponseURL)

	// 判断是否需要异步处理（文本较长或包含多条运价）
	if len(req.Query) > 100 || strings.Count(req.Query, "；") > 3 {
		// 流式响应：先返回占位消息，再异步处理
		go h.processAsync(req)
		writeJSON(w, StreamResponse{
			Content: "⏳ 运价信息较多，正在解析中，请稍候...",
			Stream:  true,
			Finish:  false,
		})
		return
	}

	// 同步处理：短文本直接返回
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	result, err := h.generateQuote(ctx, req.Query)
	if err != nil {
		log.Printf("Failed to generate quote: %v", err)
		writeJSON(w, PluginResponse{Content: fmt.Sprintf("解析失败：%v", err)})
		return
	}

	writeJSON(w, PluginResponse{Content: result})
}

// processAsync 异步处理运价解析
func (h *WebhookHandler) processAsync(req PluginRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := h.generateQuote(ctx, req.Query)
	if err != nil {
		log.Printf("[async] Failed to generate quote: %v", err)
		h.sendAsyncResult(req, fmt.Sprintf("❌ 解析失败：%v", err))
		return
	}

	h.sendAsyncResult(req, result)
}

// sendAsyncResult 发送异步处理结果
func (h *WebhookHandler) sendAsyncResult(req PluginRequest, content string) {
	if req.ResponseURL == "" {
		log.Printf("[async] No response_url provided, cannot send result")
		return
	}

	resp := PluginResponse{Content: content}
	body, err := json.Marshal(resp)
	if err != nil {
		log.Printf("[async] Failed to marshal response: %v", err)
		return
	}

	httpResp, err := h.httpClient.Post(req.ResponseURL, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[async] Failed to send result to %s: %v", req.ResponseURL, err)
		return
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		log.Printf("[async] Unexpected status code: %d", httpResp.StatusCode)
		return
	}

	log.Printf("[async] Result sent successfully to %s", req.ResponseURL)
}

// generateQuote 解析用户输入并生成 Markdown 报价单
func (h *WebhookHandler) generateQuote(ctx context.Context, userInput string) (string, error) {
	rates, err := h.llmService.ParseQuoteInput(ctx, userInput)
	if err != nil {
		return "", fmt.Errorf("解析运价信息失败: %w", err)
	}

	if len(rates) == 0 {
		return "抱歉，无法从您的输入中解析出运价信息。\n请提供航线和价格，例如：MSK 美东 SAV/NWK 2500/40HQ", nil
	}

	return formatFreightRates(rates), nil
}

// formatFreightRates 将运价数据格式化为 Markdown
func formatFreightRates(rates []llm.FreightRate) string {
	var sb strings.Builder

	sb.WriteString("**运价信息**\n\n")

	for i, r := range rates {
		if i > 0 {
			sb.WriteString("\n---\n\n")
		}

		// 船公司
		if r.Carrier != "" {
			sb.WriteString(fmt.Sprintf("**船公司**：%s\n", r.Carrier))
		}

		// 船名/航次
		if r.Vessel != "" || r.Voyage != "" {
			vessel := r.Vessel
			if r.Voyage != "" {
				vessel = vessel + "/" + r.Voyage
			}
			sb.WriteString(fmt.Sprintf("**船名/航次**：%s\n", vessel))
		}

		// 航线
		if r.POL != "" || r.POD != "" {
			sb.WriteString(fmt.Sprintf("**航线**：%s → %s\n", r.POL, r.POD))
		}

		// ETD
		if r.ETD != "" {
			sb.WriteString(fmt.Sprintf("**ETD**：%s\n", r.ETD))
		}

		// 运价
		var prices []string
		if r.F20GP != "" {
			prices = append(prices, "20GP: "+r.F20GP)
		}
		if r.F40GP != "" {
			prices = append(prices, "40GP: "+r.F40GP)
		}
		if r.F40HC != "" {
			prices = append(prices, "40HC: "+r.F40HC)
		}
		if len(prices) > 0 {
			sb.WriteString(fmt.Sprintf("**运价**：%s\n", strings.Join(prices, " / ")))
		}

		// 有效期
		if r.ValidityStartTime != "" || r.ValidityEndTime != "" {
			sb.WriteString(fmt.Sprintf("**有效期**：%s ~ %s\n", r.ValidityStartTime, r.ValidityEndTime))
		}

		// 备注
		if r.Remark != "" {
			sb.WriteString(fmt.Sprintf("**备注**：%s\n", r.Remark))
		}
	}

	return sb.String()
}
