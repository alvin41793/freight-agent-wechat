package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"freight-agent-wechat/pkg/logger"
)

// FreightRate 海运运价结构体（标准 21 字段）
type FreightRate struct {
	Agent             string `json:"Agent,omitempty"` // 代理名称
	Carrier           string `json:"Carrier,omitempty"`
	POL               string `json:"POL"`
	POLCode           string `json:"POLCode,omitempty"`
	POD               string `json:"POD"`
	PODCode           string `json:"PODCode,omitempty"`
	F20GP             string `json:"F20GP,omitempty"`
	F40GP             string `json:"F40GP,omitempty"`
	F40HC             string `json:"F40HC,omitempty"`
	ValidityStartTime string `json:"ValidityStartTime,omitempty"`
	ValidityEndTime   string `json:"ValidityEndTime,omitempty"`
	ETD               string `json:"ETD,omitempty"`
	ETA               string `json:"ETA,omitempty"`
	POLPODTT          string `json:"POLPODTT,omitempty"`
	Vessel            string `json:"Vessel,omitempty"`
	Voyage            string `json:"Voyage,omitempty"`
	CutOffTime        string `json:"Cut-off Time,omitempty"`
	PortClosingTime   string `json:"PortClosingTime,omitempty"`
	Commodity         string `json:"Commodity,omitempty"`
	DND               string `json:"DND,omitempty"`
	WeightLimit       string `json:"Weight limit,omitempty"`
	Remark            string `json:"remark,omitempty"`
}

// UnmarshalJSON 自定义反序列化，兼容模型输出数字或字符串格式的价格/航程字段
// 思考模式下模型会将 F20GP/F40GP/F40HC/POLPODTT 输出为 JSON 数字，需要转为字符串
func (f *FreightRate) UnmarshalJSON(data []byte) error {
	// type alias 避免无限递归
	type Alias FreightRate
	aux := &struct {
		F20GP    json.RawMessage `json:"F20GP,omitempty"`
		F40GP    json.RawMessage `json:"F40GP,omitempty"`
		F40HC    json.RawMessage `json:"F40HC,omitempty"`
		POLPODTT json.RawMessage `json:"POLPODTT,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(f),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	f.F20GP = jsonNumOrStr(aux.F20GP)
	f.F40GP = jsonNumOrStr(aux.F40GP)
	f.F40HC = jsonNumOrStr(aux.F40HC)
	f.POLPODTT = jsonNumOrStr(aux.POLPODTT)
	return nil
}

// jsonNumOrStr 将 JSON 原始值转为字符串，兼容 JSON 数字和字符串两种格式
func jsonNumOrStr(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return ""
}

// Service LLM 服务
type Service struct {
	client      *Client
	log         *logger.Logger // 可为 nil（pro 模式）
	promptStore PromptStore    // 提示词存储，可为 nil
}

// PromptStore 提示词存储接口
type PromptStore interface {
	GetActivePrompt(name string) (string, error)
}

// NewService 创建 LLM 服务（无调试日志）
func NewService(client *Client) *Service {
	return &Service{client: client}
}

// NewServiceWithLogger 创建 LLM 服务（debug 模式下记录输入输出到文件）
func NewServiceWithLogger(client *Client, log *logger.Logger) *Service {
	return &Service{client: client, log: log}
}

// WithPromptStore 设置提示词存储
func (s *Service) WithPromptStore(store PromptStore) *Service {
	s.promptStore = store
	return s
}

// getSystemPrompt 获取系统提示词（优先从数据库获取，失败则使用默认值）
func (s *Service) getSystemPrompt(name string, defaultPrompt string) string {
	if s.promptStore == nil {
		return defaultPrompt
	}

	content, err := s.promptStore.GetActivePrompt(name)
	if err != nil || content == "" {
		return defaultPrompt
	}

	return content
}

const ExtractSystemPromptDefault = `你是海运运价结构化解析专家。

# 任务
从输入的非结构化文本中，提取"可确定"的海运运价信息，并输出标准 JSON 数组。

⚠️ 核心原则：
1. 只提取明确关联的信息，禁止猜测、禁止跨段拼接。
2. 宁可少输出，不输出错误数据。
3. 严格区分卖价与收货价/成本。

--------------------------------
# 零、最高优先级规则

## POL 起运港提取规则

✅ 允许提取 POL 的情况：
1. 明确标注："XX出"、"XX-YY"、港口列表（如"青岛/上海/宁波"）
2. 公司名中的港口名："宁波弘泰"→NINGBO，"上海铮航"→SHANGHAI，"深圳均辉"→SHENZHEN，"青岛欣皓"→QINGDAO
3. 同一段落中其他行明确提到

❌ 禁止提取 POL 的情况：
- 原文完全没有提及任何起运港（包括公司名中也没有）
- 只有船名、航次、价格，没有起运港信息

🚫 禁止主动填充默认值：NINGBO, SHANGHAI, QINGDAO 等（除非原文明确提及）

## Carrier 承运人继承规则

在一个报价块内，承运人信息在首次出现后，后续行自动继承。禁止输出"未知船公司"，无法确定时省略 Carrier 字段。

## Agent 代理名称提取规则

从文本开头提取货代/代理公司名称，作为 Agent 字段输出。

识别模式：
- 序号后的公司名：如"1.宁波弘泰" → Agent = "宁波弘泰"
- 行首的公司名：如"宁波弘泰"、"上海铮航"、"深圳均辉"
- 公司名通常位于报价块的第一行

--------------------------------
# 一、输出格式要求
1. 仅输出纯 JSON 数组，不要任何额外文字。无法提取时输出 []。
2. 每条记录必须包含：POL + POD + 运价
3. 缺失字段直接省略

--------------------------------
# 二、字段定义
Agent（代理名称,如"宁波弘泰"、"上海铮航"）, Carrier, POL, POLCode, POD, PODCode, F20GP, F40GP, F40HC, ValidityStartTime, ValidityEndTime, ETD, Vessel, Voyage, Cut-off Time, PortClosingTime, Commodity, DND, Weight limit, remark

--------------------------------
# 三、关键解析规则

## 1️⃣ 价格映射
- "2700/3000" → F20GP=2700, F40GP=3000
- "1775/2215/2215" → F20GP=1775, F40GP=2215, F40HC=2215
- 第4个及以后数字 → 写入 remark
- NOR价格 → 写入 remark，Commodity="NOR"
- 后缀 ++ / +A / 含附加费 → 写入 remark

## 2️⃣ 港口映射
- 美西 → LOS ANGELES(USLAX)
- 美东 → NEW YORK(USNYC)
- 美湾 → HOUSTON(USHOU)
- 内陆点(CHI/DAL/MEM) → 作为 POD 输出

## 3️⃣ 转运路径解析
- "CAL/EDM via VAN" → POD=CALGARY/EDMONTON, remark="via VANCOUVER"
- "MTR via VAN" → POD=MONTREAL
- "TOR via VAN" → POD=TORONTO
- "Minneapolis via TIW" → POD=MINNEAPOLIS
- "ia" 视为 "via" 的笔误

## 4️⃣ 日期规则
- 年份默认 2026，格式 yyyy/MM/dd
- 进港有效期范围 → ValidityStartTime/ValidityEndTime
- ETD → 预计开航日期

## 5️⃣ 港口代码（常用）
中国：NINGBO(CNNGB), SHANGHAI(CNSHA), SHENZHEN(CNSZX), QINGDAO(CNQDG), XIAMEN(CNXMN), TIANJIN(CNTSN)
美国：LOS ANGELES(USLAX), LONG BEACH(USLGB), OAKLAND(USOAK), TACOMA(USTIW), NEW YORK(USNYC), SAVANNAH(USSAV), HOUSTON(USHOU), CHICAGO(USCHI)
加拿大：VANCOUVER(CAVAN), TORONTO(CATOR), MONTREAL(CAMTR), CALGARY(CACAL), EDMONTON(CAEDM)

--------------------------------
# 四、输出示例

示例1：正常输入
输入：1.宁波弘泰 MSK 宁波出 ETD4.14 ONE YM MASCULINITY 105E VAN 1750/2300/2300/2550++ 以上4.9-4.14进港有效
输出：[{"Agent":"宁波弘泰","Carrier":"ONE","POL":"NINGBO","POLCode":"CNNGB","POD":"VANCOUVER","PODCode":"CAVAN","F20GP":"1750","F40GP":"2300","F40HC":"2300","ETD":"2026/04/14","Vessel":"YM MASCULINITY","Voyage":"105E","ValidityStartTime":"2026/04/09","ValidityEndTime":"2026/04/14","remark":"45HQ:2550++"}]

示例2：转运路径
输入：宁波弘泰 CAL/EDM via VAN 3000/3750/3750/4600++
输出：[{"Agent":"宁波弘泰","POD":"CALGARY","PODCode":"CACAL","F20GP":"3000","F40GP":"3750","F40HC":"3750","remark":"via VANCOUVER 4600++"},{"Agent":"宁波弘泰","POD":"EDMONTON","PODCode":"CAEDM","F20GP":"3000","F40GP":"3750","F40HC":"3750","remark":"via VANCOUVER 4600++"}]

示例3：无POL输入
输入：ETD4.14 ONE YM MASCULINITY 105E VAN 1750/2300/2300
输出：[]

--------------------------------
# 五、开始解析

请解析以下文本，仅输出 JSON 数组：

`

// SupplementSystemPromptDefault 默认的运价数据补全提示词模板（包含 %s 占位符）
const SupplementSystemPromptDefault = `你是海运运价数据补全助手。用户之前提供了一批运价数据，但部分记录缺少必要字段，现在用户补充了相关信息。

当前待完善的运价记录（JSON格式）：
%s

缺失字段说明：
%s

请根据用户新提供的补充信息，填充上述缺失字段，输出完整的 JSON 数组。规则：
1. 仅修改缺失的字段，其他字段保持不变
2. 日期格式统一为 yyyy/MM/dd，当前年份为 2026 年
3. 价格字段只保留数字
4. 输出完整的 JSON 数组，包含所有记录（包括已完整和待补充的）`

// extractUserChat 从 context 中提取用户和会话 ID（用于日志记录）
func extractUserChat(ctx context.Context) (userID, chatID string) {
	if v := ctx.Value(logger.ContextKeyUserID); v != nil {
		userID, _ = v.(string)
	}
	if v := ctx.Value(logger.ContextKeyChatID); v != nil {
		chatID, _ = v.(string)
	}
	return
}

// ParseQuoteInput 解析用户输入的运价文本，返回结构化运价数组
// debug 模式下将用户输入和模型输出记录到 logs/{date}/llm_{date}.jsonl
func (s *Service) ParseQuoteInput(ctx context.Context, userInput string) ([]FreightRate, error) {
	start := time.Now()
	userPrompt := fmt.Sprintf("%s\n\n请开始提取，输出 JSON。", userInput)

	// 从数据库或默认值获取提示词
	systemPrompt := s.getSystemPrompt("extract_system_prompt", ExtractSystemPromptDefault)

	var rates []FreightRate
	reasoningContent, err := s.client.CompleteWithJSON(ctx, &CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	}, &rates)

	// debug 日志
	if s.log != nil && s.log.IsDebug() {
		entry := &logger.LLMEntry{
			Time:             time.Now(),
			Function:         "ParseQuoteInput",
			SystemPrompt:     systemPrompt, // 完整系统提示词
			UserInput:        userInput,
			ReasoningContent: reasoningContent,
			DurationMS:       time.Since(start).Milliseconds(),
		}
		entry.UserID, entry.ChatID = extractUserChat(ctx)
		if err != nil {
			entry.Error = err.Error()
		} else if out, e := json.Marshal(rates); e == nil {
			entry.Output = string(out)
		}
		// Debug 模式: 写入 TXT 完整日志
		s.log.LogLLMDebug(entry)
	}

	return rates, err
}

// SupplementRates 用新输入补充待完善的运价记录的缺失字段
// debug 模式下记录补充输入和合并后的输出
func (s *Service) SupplementRates(ctx context.Context, pendingRates []FreightRate, missingFields map[int][]string, userInput string) ([]FreightRate, error) {
	start := time.Now()
	pendingJSON, _ := json.MarshalIndent(pendingRates, "", "  ")

	// 构建缺失字段描述
	missingDesc := ""
	for idx, fields := range missingFields {
		if idx < len(pendingRates) {
			r := pendingRates[idx]
			missingDesc += fmt.Sprintf("第 %d 条（%s→%s）缺少：%v\n", idx+1, r.POL, r.POD, fields)
		}
	}

	systemPromptTemplate := s.getSystemPrompt("supplement_system_prompt", SupplementSystemPromptDefault)
	systemPrompt := fmt.Sprintf(systemPromptTemplate, string(pendingJSON), missingDesc)

	userPrompt := fmt.Sprintf("补充信息：%s", userInput)

	var rates []FreightRate
	reasoningContent, err := s.client.CompleteWithJSON(ctx, &CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	}, &rates)

	if s.log != nil && s.log.IsDebug() {
		entry := &logger.LLMEntry{
			Time:             time.Now(),
			Function:         "SupplementRates",
			SystemPrompt:     systemPrompt, // 完整系统提示词
			UserInput:        userInput,
			Context:          string(pendingJSON), // 附上原始待补充运价
			ReasoningContent: reasoningContent,
			DurationMS:       time.Since(start).Milliseconds(),
		}
		entry.UserID, entry.ChatID = extractUserChat(ctx)
		if err != nil {
			entry.Error = err.Error()
		} else if out, e := json.Marshal(rates); e == nil {
			entry.Output = string(out)
		}
		// Debug 模式: 写入 TXT 完整日志
		s.log.LogLLMDebug(entry)
	}

	return rates, err
}
