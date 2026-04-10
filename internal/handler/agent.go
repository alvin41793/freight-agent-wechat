package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"freight-agent-wechat/internal/llm"
	"freight-agent-wechat/internal/push"
	"freight-agent-wechat/internal/session"
	"freight-agent-wechat/internal/store"
	"freight-agent-wechat/pkg/logger"
)

// normalizeText 规范化用户输入文本，消除企业微信/复制粘贴引入的特殊 Unicode 字符
func normalizeText(s string) string {
	// \u00a0 不间断空格 → 普通空格
	s = strings.ReplaceAll(s, "\u00a0", " ")
	// \u200b 零宽空格 → 删除
	s = strings.ReplaceAll(s, "\u200b", "")
	// \u3000 全角空格 → 普通空格
	s = strings.ReplaceAll(s, "\u3000", " ")
	return strings.TrimSpace(s)
}

// rePrice 匹配 3-5 位数字（常见海运运费价格区间）
var rePrice = regexp.MustCompile(`\b\d{3,5}\b`)

// freightCarriers 已知船公司关键词
var freightCarriers = []string{
	// 中文名称
	"万海", "长荣", "阳明", "海丰", "中远", "太仓", "南星", "开远", "东方", "马士基",
	// 英文缩写/全名
	"MSK", "MAERSK", "MSC", "COSCO", "CMA", "OOCL", "HPL", "ONE", "EMC", "PIL", "ZIM", "SITC",
	"HMM", "YML", "WHL", "WANHAI", "ANL", "APL", "KMTC", "MAT", "TS", "SM",
	"EVERGREEN", "YANGMING", "HAPAG", "HLCU", "HAMBURG", "TS LINES",
}

// freightContainerTypes 箱型标识
var freightContainerRe = regexp.MustCompile(`(?i)\b(20GP|40GP|40HC|45HC|20HQ|40HQ|45HQ|20OT|40OT|20FR|40FR|20RF|40RF)\b`)

// freightDirections 航线/港口方向关键词
var freightDirections = []string{
	"美西", "美东", "欧洲", "欧基", "地中海", "东南亚", "南美", "中东", "非洲", "澳洲",
	"LAX", "LGB", "OAK", "SEA", "NYC", "NFK", "SAV", "HOU", "MIA",
	"HAM", "RTM", "FEL", "ANR", "GDN", "PIR",
	"SHA", "NGB", "XMN", "CAN", "QIN", "TXG",
}

// isLikelyFreightInput 判断输入是否可能是运价相关文本（轻量规则判断，不发起 LLM 调用）
// 命中一个以上信号则认为是运价文本
func isLikelyFreightInput(text string) bool {
	// 1. 包含箱型标识（最确切的信号）
	if freightContainerRe.MatchString(text) {
		return true
	}

	upper := strings.ToUpper(text)

	// 2. 包含已知船公司名
	for _, c := range freightCarriers {
		if strings.Contains(text, c) || strings.Contains(upper, strings.ToUpper(c)) {
			return true
		}
	}

	// 3. 包含价格数字 + 航线方向词
	if rePrice.MatchString(text) {
		for _, d := range freightDirections {
			if strings.Contains(text, d) || strings.Contains(upper, strings.ToUpper(d)) {
				return true
			}
		}
	}

	return false
}

// FreightAgentHandler 运价 Agent 主处理器，实现 bot.MessageHandler 接口
type FreightAgentHandler struct {
	llmService   *llm.Service
	rateStore    *store.FreightRateStore
	taskStore    *store.TaskStore
	pendingStore *session.PendingStore
	pushService  *push.PushService
	logger       *logger.Logger
}

// NewFreightAgentHandler 创建运价 Agent 处理器
func NewFreightAgentHandler(
	llmSvc *llm.Service,
	rateStore *store.FreightRateStore,
	taskStore *store.TaskStore,
	pendingStore *session.PendingStore,
	pushSvc *push.PushService,
) *FreightAgentHandler {
	return &FreightAgentHandler{
		llmService:   llmSvc,
		rateStore:    rateStore,
		taskStore:    taskStore,
		pendingStore: pendingStore,
		pushService:  pushSvc,
	}
}

// WithLogger 设置日志记录器
func (h *FreightAgentHandler) WithLogger(l *logger.Logger) *FreightAgentHandler {
	h.logger = l
	return h
}

// Handle 实现 bot.MessageHandler 接口，处理用户消息
func (h *FreightAgentHandler) Handle(ctx context.Context, userID, chatID, text string) string {
	if text == "" {
		return ""
	}
	text = normalizeText(text)

	if h.logger != nil {
		h.logger.Info("[agent] received message: user=%s chat=%s", userID, chatID)
	} else {
		log.Printf("[agent] user=%s chat=%s text=%q", userID, chatID, text)
	}

	// 将用户/会话信息写入 context，供 LLM 服务 debug 日志使用
	ctx = context.WithValue(ctx, logger.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, logger.ContextKeyChatID, chatID)

	// 创建任务记录
	taskStart := time.Now()
	taskID := store.NewTaskID()
	taskStatus := store.TaskStatusCompleted // 默认成功，出错时修改
	task := &store.TaskRecord{
		ID:      taskID,
		UserID:  userID,
		ChatID:  chatID,
		RawText: text,
		Status:  store.TaskStatusPending,
	}
	if err := h.taskStore.Create(ctx, task); err != nil {
		if h.logger != nil {
			h.logger.Error("[agent] create task error: %v", err)
		} else {
			log.Printf("[agent] create task error: %v", err)
		}
	}
	defer func() {
		h.taskStore.Complete(ctx, taskID, taskStatus, time.Since(taskStart).Milliseconds())
	}()

	// ① 意图预判断：输入是否运价相关
	isFreight := isLikelyFreightInput(text)
	if !isFreight {
		taskStatus = store.TaskStatusRejected
		if h.logger != nil {
			h.logger.Info("[agent] non-freight input, skip parsing")
		} else {
			log.Printf("[agent] non-freight input, skip parsing")
		}
		return "我是运价分析机器人，可以帮您从运价报价文本中提取结构化数据。\n\n请发送包含以下信息的运价文本：\n• 船公司名称（如万海、MSC、COSCO）\n• 起运港 / 目的港\n• 箱型价格（20GP / 40GP / 40HC）\n• 有效期"
	}

	// ② LLM 运价提取
	llmStart := time.Now()
	rates, err := h.llmService.ParseQuoteInput(ctx, text)
	llmMs := time.Since(llmStart).Milliseconds()
	if h.logger != nil {
		h.logger.Info("[agent] LLM parse completed: elapsed=%dms rates=%d", llmMs, len(rates))
	} else {
		log.Printf("[agent] ParseQuoteInput elapsed=%s", time.Since(llmStart).Round(time.Millisecond))
	}
	if err != nil {
		taskStatus = store.TaskStatusFailed
		h.taskStore.AddStep(ctx, &store.TaskStepRecord{
			TaskID:     taskID,
			StepType:   store.StepTypeParse,
			Status:     store.StepStatusFailed,
			Summary:    fmt.Sprintf("LLM 解析失败，耗时 %dms", llmMs),
			Error:      err.Error(),
			DurationMS: store.Int64Ptr(llmMs),
		})
		if h.logger != nil {
			h.logger.Error("[agent] LLM parse error: %v", err)
		} else {
			log.Printf("[agent] parse quote error: %v", err)
		}
		// 检查是否为第三方API错误，如果是则返回友好提示
		userErrMsg := formatLLMErrorForUser(err)
		return userErrMsg
	}

	// 记录模型输出的 JSON
	if modelJSON, err := json.Marshal(rates); err == nil {
		task.ModelOutputJSON = string(modelJSON)
	}

	h.taskStore.AddStep(ctx, &store.TaskStepRecord{
		TaskID:     taskID,
		StepType:   store.StepTypeParse,
		Status:     store.StepStatusSuccess,
		Summary:    fmt.Sprintf("解析到 %d 条运价，耗时 %dms", len(rates), llmMs),
		DurationMS: store.Int64Ptr(llmMs),
	})
	if len(rates) == 0 {
		return "未能从您的输入中提取到运价信息。\n\n请提供包含起运港、目的港、运价的运价文本。"
	}

	// ③ 字段校验
	valid, invalid := filterValidRates(rates)
	if h.logger != nil {
		h.logger.Info("[agent] validation: total=%d valid=%d invalid=%d", len(rates), len(valid), len(invalid))
	} else {
		log.Printf("[agent] rates total=%d valid=%d invalid=%d", len(rates), len(valid), len(invalid))
	}
	if len(valid) == 0 {
		return buildInvalidOnlyReply(invalid)
	}

	// ④ 保存有效运价
	saveStart := time.Now()
	savedIDs, saveErr := h.rateStore.BatchSave(ctx, valid, userID, chatID)
	saveMs := time.Since(saveStart).Milliseconds()
	if saveErr != nil {
		taskStatus = store.TaskStatusFailed
		h.taskStore.AddStep(ctx, &store.TaskStepRecord{
			TaskID:     taskID,
			StepType:   store.StepTypeSave,
			Status:     store.StepStatusFailed,
			Summary:    fmt.Sprintf("保存 %d 条运价失败，耗时 %dms", len(valid), saveMs),
			Error:      saveErr.Error(),
			DurationMS: store.Int64Ptr(saveMs),
		})
		if h.logger != nil {
			h.logger.Error("[agent] save rates error: %v", saveErr)
		} else {
			log.Printf("[agent] save rates error: %v", saveErr)
		}
		return fmt.Sprintf("⚠️ 运价保存失败：%v", saveErr)
	}

	// 记录保存到数据库的数据 JSON
	if savedJSON, err := json.Marshal(valid); err == nil {
		task.SavedDataJSON = string(savedJSON)
	}

	h.taskStore.AddStep(ctx, &store.TaskStepRecord{
		TaskID:     taskID,
		StepType:   store.StepTypeSave,
		Status:     store.StepStatusSuccess,
		Summary:    fmt.Sprintf("成功保存 %d 条运价，耗时 %dms", len(valid), saveMs),
		DurationMS: store.Int64Ptr(saveMs),
	})

	// ⑤ 推送到第三方(如果启用)
	var pushResult *push.PushResult
	if h.pushService != nil && h.pushService.IsEnabled() {
		pushStart := time.Now()
		result := h.pushService.PushRates(ctx, valid)
		pushMs := time.Since(pushStart).Milliseconds()
		pushResult = &result
		if h.logger != nil {
			h.logger.Info("[agent] push completed: total=%d success=%d failed=%d skipped=%d elapsed=%dms",
				result.Total, result.Success, result.Failed, result.Skipped, pushMs)
		} else {
			log.Printf("[agent] push rates: total=%d success=%d failed=%d skipped=%d elapsed=%s",
				result.Total, result.Success, result.Failed, result.Skipped,
				time.Since(pushStart).Round(time.Millisecond))
		}

		// 更新推送状态
		if len(savedIDs) > 0 {
			// 根据推送结果中的索引，分别更新成功和失败的记录
			var successIDs []uint64
			var failedIDs []uint64

			// 构建成功和失败的 ID 列表
			for _, idx := range result.SuccessIndices {
				if idx < len(savedIDs) {
					successIDs = append(successIDs, savedIDs[idx])
				}
			}
			for _, idx := range result.FailedIndices {
				if idx < len(savedIDs) {
					failedIDs = append(failedIDs, savedIDs[idx])
				}
			}

			// 构建错误信息
			errorMsg := ""
			if len(result.Failures) > 0 {
				// 提取第一条失败记录的错误信息
				errorMsg = result.Failures[0].Error
			}

			// 更新数据库状态
			if err := h.rateStore.UpdatePushStatus(ctx, successIDs, failedIDs, errorMsg); err != nil {
				if h.logger != nil {
					h.logger.Error("[agent] update push status error: %v", err)
				} else {
					log.Printf("[agent] update push status error: %v", err)
				}
			}
		}

		// 记录推送请求和响应 JSON（保存第三方API的原始请求和响应）
		if pushResult.RawAPIRequest != "" {
			task.PushRequestJSON = pushResult.RawAPIRequest
		}
		if pushResult.RawAPIResponse != "" {
			task.PushResponseJSON = pushResult.RawAPIResponse
		} else {
			// 如果没有原始API响应，则保存内部统计信息
			pushResponse := map[string]interface{}{
				"total":    result.Total,
				"success":  result.Success,
				"failed":   result.Failed,
				"skipped":  result.Skipped,
				"failures": result.Failures,
			}
			if pushRespJSON, err := json.Marshal(pushResponse); err == nil {
				task.PushResponseJSON = string(pushRespJSON)
			}
		}

		h.taskStore.AddStep(ctx, &store.TaskStepRecord{
			TaskID:   taskID,
			StepType: store.StepTypePush,
			Status: func() string {
				if result.Failed == 0 && result.Skipped == 0 {
					return store.StepStatusSuccess
				}
				if result.Success == 0 {
					return store.StepStatusFailed
				}
				return store.StepStatusPartial
			}(),
			Summary:    fmt.Sprintf("推送完成：成功 %d, 失败 %d, 跳过 %d，耗时 %dms", result.Success, result.Failed, result.Skipped, pushMs),
			DurationMS: store.Int64Ptr(pushMs),
		})
	}

	// 更新任务记录中的关键字段
	if err := h.taskStore.UpdateTaskData(ctx, taskID, task.ModelOutputJSON, task.SavedDataJSON, task.PushRequestJSON, task.PushResponseJSON); err != nil {
		if h.logger != nil {
			h.logger.Error("[agent] update task data error: %v", err)
		}
	}

	return buildRateTableReply(valid, invalid, pushResult)
}

// colDef 定义表格列的展示名称和取值方式
type colDef struct {
	header   string
	getValue func(r llm.FreightRate) string
}

// rateColumns 所有可展示列（按常见程度排序）——目的港始终作为第一列
var rateColumns = []colDef{
	{"20GP", func(r llm.FreightRate) string { return r.F20GP }},
	{"40GP", func(r llm.FreightRate) string { return r.F40GP }},
	{"40HC", func(r llm.FreightRate) string { return r.F40HC }},
	{"有效期", func(r llm.FreightRate) string {
		if r.ValidityStartTime != "" && r.ValidityEndTime != "" {
			return r.ValidityStartTime + "~" + r.ValidityEndTime
		} else if r.ValidityStartTime != "" {
			return r.ValidityStartTime + "~"
		} else if r.ValidityEndTime != "" {
			return "~" + r.ValidityEndTime
		}
		return ""
	}},
	{"ETD", func(r llm.FreightRate) string { return r.ETD }},
	{"ETA", func(r llm.FreightRate) string { return r.ETA }},
	{"航程(天)", func(r llm.FreightRate) string { return r.POLPODTT }},
	{"船名", func(r llm.FreightRate) string { return r.Vessel }},
	{"航次", func(r llm.FreightRate) string { return r.Voyage }},
	{"截关", func(r llm.FreightRate) string { return r.CutOffTime }},
	{"集港", func(r llm.FreightRate) string { return r.PortClosingTime }},
	{"品名", func(r llm.FreightRate) string { return r.Commodity }},
	{"免笱免堆", func(r llm.FreightRate) string { return r.DND }},
	{"限重", func(r llm.FreightRate) string { return r.WeightLimit }},
	{"备注", func(r llm.FreightRate) string {
		v := r.Remark
		if v == "" {
			return ""
		}
		runes := []rune(v)
		if len(runes) > 30 {
			return string(runes[:30]) + "…"
		}
		return v
	}},
}

// buildRateTableReply 构建运价列表回复，按 Agent+Carrier+POL 分组，每组输出一个 Markdown 表格。
// 列动态根据实际提取到的字段决定，没有内容的列不展示。
func buildRateTableReply(rates []llm.FreightRate, invalid []llm.FreightRate, pushResult *push.PushResult) string {
	var sb strings.Builder

	// 推送结果（如果有）
	if pushResult != nil && (pushResult.Success > 0 || pushResult.Failed > 0) {
		sb.WriteString(fmt.Sprintf("📤 推送结果: 成功 %d 条, 失败 %d 条", pushResult.Success, pushResult.Failed))
		if pushResult.Skipped > 0 {
			sb.WriteString(fmt.Sprintf(", 跳过 %d 条(缺少必填字段)", pushResult.Skipped))
		}
		sb.WriteString("\n")

		// 如果有失败,显示详细错误（只取 msg 字段）
		if len(pushResult.Failures) > 0 {
			// 提取第一条失败的错误信息中的 msg 部分
			errorMsg := extractPushErrorMsg(pushResult.Failures[0].Error)
			sb.WriteString(fmt.Sprintf("❌ %s\n", errorMsg))
		}
		sb.WriteString("\n")
	}

	// 按 Agent+Carrier+POL 分组，保留插入顺序
	type groupKey struct{ Agent, Carrier, POL string }
	var order []groupKey
	groups := map[groupKey][]llm.FreightRate{}
	for _, r := range rates {
		k := groupKey{r.Agent, r.Carrier, r.POL}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}

	for gi, key := range order {
		group := groups[key]
		if gi > 0 {
			sb.WriteString("\n")
		}

		// 组标题：Agent（如果有）+ Carrier | POL(POLCode)
		var titleParts []string
		if key.Agent != "" {
			titleParts = append(titleParts, key.Agent)
		}
		carrier := key.Carrier
		if carrier == "" {
			carrier = "未知船公司"
		}
		titleParts = append(titleParts, carrier)

		pol := key.POL
		if pol == "" {
			pol = "未知起运港"
		}
		if polCode := group[0].POLCode; polCode != "" {
			pol = pol + "(" + polCode + ")"
		}
		titleParts = append(titleParts, pol)

		sb.WriteString(fmt.Sprintf("\n**%s**\n\n", strings.Join(titleParts, " | ")))

		// 扫描该组有实际数据的列
		var activeCols []colDef
		for _, col := range rateColumns {
			for _, r := range group {
				if col.getValue(r) != "" {
					activeCols = append(activeCols, col)
					break
				}
			}
		}

		// 表头：目的港 + 动态列 + 推送状态
		sb.WriteString("| 目的港")
		for _, col := range activeCols {
			sb.WriteString(" | " + col.header)
		}
		sb.WriteString(" | 推送状态 |\n")

		// 分隔行
		sb.WriteString("|---")
		for range activeCols {
			sb.WriteString("|---")
		}
		sb.WriteString("|---|\n")

		// 数据行
		for _, r := range group {
			pod := r.POD
			if pod == "" {
				pod = "-"
			} else if r.PODCode != "" {
				pod = pod + "(" + r.PODCode + ")"
			}
			sb.WriteString("| " + pod)
			for _, col := range activeCols {
				v := col.getValue(r)
				if v == "" {
					v = "-"
				}
				sb.WriteString(" | " + v)
			}
			// 推送状态列（默认为空，因为这是实时展示，还没推送）
			sb.WriteString(" | - |\n")
		}
	}

	// 附加无效运价提示
	if len(invalid) > 0 {
		sb.WriteString(fmt.Sprintf("\n\n⚠️ 另有 %d 条因缺少必要字段未保存：\n", len(invalid)))
		for i, r := range invalid {
			pol := r.POL
			if pol == "" {
				pol = "?"
			}
			pod := r.POD
			if pod == "" {
				pod = "?"
			}
			sb.WriteString(fmt.Sprintf("%d. %s→%s 缺：%s\n", i+1, pol, pod, strings.Join(missingFields(r), "、")))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// filterValidRates 过滤有效运价。
// 有效条件：POL + POD + 有效期（至少一端）+ 价格（20GP/40GP/40HC 至少一个非空且非“0”）
func filterValidRates(rates []llm.FreightRate) (valid, invalid []llm.FreightRate) {
	for _, r := range rates {
		switch {
		case r.POL == "":
			invalid = append(invalid, r)
		case r.POD == "":
			invalid = append(invalid, r)
		case r.ValidityStartTime == "" || r.ValidityEndTime == "":
			invalid = append(invalid, r)
		case !hasValidPrice(r):
			invalid = append(invalid, r)
		default:
			valid = append(valid, r)
		}
	}
	return
}

// hasValidPrice 判断运价是否有至少一个有效价格（非空、非“0”、非“0.00”）
func hasValidPrice(r llm.FreightRate) bool {
	for _, v := range []string{r.F20GP, r.F40GP, r.F40HC} {
		if v != "" && v != "0" && v != "0.00" {
			return true
		}
	}
	return false
}

// buildInvalidOnlyReply 全部为无效运价时的回复
func buildInvalidOnlyReply(invalid []llm.FreightRate) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚠️ 解析到 %d 条运价，但均缺少必要字段（起运港/目的港/有效期/价格），未保存。\n\n缺少内容：\n", len(invalid)))
	for i, r := range invalid {
		missing := missingFields(r)
		pol := r.POL
		if pol == "" {
			pol = "?"
		}
		pod := r.POD
		if pod == "" {
			pod = "?"
		}
		sb.WriteString(fmt.Sprintf("%d. %s→%s 缺：%s\n", i+1, pol, pod, strings.Join(missing, "、")))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// missingFields 返回缺少的字段名称
func missingFields(r llm.FreightRate) []string {
	var m []string
	if r.POL == "" {
		m = append(m, "起运港")
	}
	if r.POD == "" {
		m = append(m, "目的港")
	}
	if r.ValidityStartTime == "" && r.ValidityEndTime == "" {
		m = append(m, "有效期")
	}
	if !hasValidPrice(r) {
		m = append(m, "价格")
	}
	return m
}

// buildPriceStr 将运价字段拼成展示字符串
func buildPriceStr(r llm.FreightRate) string {
	var parts []string
	if r.F20GP != "" {
		parts = append(parts, "20GP: "+r.F20GP)
	}
	if r.F40GP != "" {
		parts = append(parts, "40GP: "+r.F40GP)
	}
	if r.F40HC != "" {
		parts = append(parts, "40HC: "+r.F40HC)
	}
	return strings.Join(parts, " / ")
}

// extractPushErrorMsg 从推送错误信息中提取 msg 字段
// 错误信息格式："api error: code=50000, msg=批量新增异常 第1条数据：..."
// 如果 msg 为空，返回“未知错误”
func extractPushErrorMsg(errMsg string) string {
	if errMsg == "" {
		return "未知错误"
	}

	// 尝试提取 msg= 后面的内容
	if idx := strings.Index(errMsg, "msg="); idx != -1 {
		msg := errMsg[idx+4:]
		if msg == "" {
			return "未知错误"
		}
		return msg
	}

	// 如果没有 msg= 前缀，直接返回原错误信息
	if errMsg == "" {
		return "未知错误"
	}
	return errMsg
}

// formatLLMErrorForUser 将LLM错误格式化为用户友好的错误消息
// 如果是第三方API错误（如网络超时、认证失败等），返回通用提示
// 否则返回原始错误信息
func formatLLMErrorForUser(err error) string {
	if err == nil {
		return "运价解析失败\n\n请检查格式后重试。"
	}

	errMsg := err.Error()
	errLower := strings.ToLower(errMsg)

	// 检测是否为第三方API相关错误
	isAPIError := strings.Contains(errLower, "chat completion failed") ||
		strings.Contains(errLower, "post") ||
		strings.Contains(errLower, "context deadline exceeded") ||
		strings.Contains(errLower, "connection refused") ||
		strings.Contains(errLower, "timeout") ||
		strings.Contains(errLower, "network") ||
		strings.Contains(errLower, "dashscope") ||
		strings.Contains(errLower, "aliyuncs") ||
		strings.Contains(errLower, "openai") ||
		strings.Contains(errLower, "api error") ||
		strings.Contains(errLower, "401") ||
		strings.Contains(errLower, "403") ||
		strings.Contains(errLower, "500") ||
		strings.Contains(errLower, "502") ||
		strings.Contains(errLower, "503") ||
		strings.Contains(errLower, "504")

	if isAPIError {
		return "网络问题，请稍后重试"
	}

	// 其他错误返回原始信息
	return fmt.Sprintf("运价解析失败：%v\n\n请检查格式后重试。", err)
}
