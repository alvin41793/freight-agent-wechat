package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

// Intent 用户意图
type Intent string

const (
	IntentCreateQuote Intent = "create_quote" // 创建报价单
	IntentUpdateQuote Intent = "update_quote" // 修改报价单
	IntentExplain     Intent = "explain"      // 解释当前报价
	IntentExport      Intent = "export"       // 导出报价单
	IntentRollback    Intent = "rollback"     // 回滚版本
	IntentClear       Intent = "clear"        // 清空会话
	IntentUnknown     Intent = "unknown"      // 未知意图
)

// IntentResult 意图识别结果
type IntentResult struct {
	Intent       Intent  `json:"intent"`
	Confidence   float64 `json:"confidence"`
	Message      string  `json:"message"`                 // 给用户的回复
	ExportFormat string  `json:"export_format,omitempty"` // export 时的格式: image, excel, text
}

// RecognizeIntent 识别用户意图
func (s *Service) RecognizeIntent(ctx context.Context, userInput string, hasActiveQuote bool) (*IntentResult, error) {
	systemPrompt := `你是一个货运报价单系统的意图识别助手。

请分析用户的输入，识别其意图并返回 JSON 格式结果。

可能的意图：
- create_quote: 创建新的报价单（包含航线、价格等信息）
- update_quote: 修改现有报价单（如改价格、添加箱型等）
- explain: 询问或解释当前报价详情
- export: 导出/发送报价单（指定格式：图片、Excel、文本）
- rollback: 回滚到之前的版本
- clear: 清空当前会话/开始新的报价
- unknown: 无法识别的意图

当前状态：` + fmt.Sprintf("%v", hasActiveQuote) + `

输出格式：
{
  "intent": "意图名称",
  "confidence": 0.0-1.0,
  "message": "给用户的友好回复",
  "export_format": "image/excel/text" // 仅在 export 意图时
}`

	userPrompt := fmt.Sprintf("用户输入：%s", userInput)

	var result IntentResult
	if _, err := s.client.CompleteWithJSON(ctx, &CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	}, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ParseExportFormat 解析导出格式
func ParseExportFormat(format string) string {
	switch format {
	case "excel", "xlsx", "xls":
		return "excel"
	case "text", "txt", "markdown", "md":
		return "text"
	case "image", "img", "picture", "pic", "":
		return "image"
	default:
		return "image"
	}
}

// IntentResultJSON 用于 JSON 序列化的结构
type IntentResultJSON struct {
	Intent       string  `json:"intent"`
	Confidence   float64 `json:"confidence"`
	Message      string  `json:"message"`
	ExportFormat string  `json:"export_format,omitempty"`
}

// ToJSON 转换为 JSON
func (r *IntentResult) ToJSON() ([]byte, error) {
	return json.Marshal(IntentResultJSON{
		Intent:       string(r.Intent),
		Confidence:   r.Confidence,
		Message:      r.Message,
		ExportFormat: r.ExportFormat,
	})
}
