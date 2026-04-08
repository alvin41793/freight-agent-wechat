package handler

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"freight-agent-wechat/internal/llm"
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
}

// NewFreightAgentHandler 创建运价 Agent 处理器
func NewFreightAgentHandler(
	llmSvc *llm.Service,
	rateStore *store.FreightRateStore,
	taskStore *store.TaskStore,
	pendingStore *session.PendingStore,
) *FreightAgentHandler {
	return &FreightAgentHandler{
		llmService:   llmSvc,
		rateStore:    rateStore,
		taskStore:    taskStore,
		pendingStore: pendingStore,
	}
}

// Handle 实现 bot.MessageHandler 接口，处理用户消息
func (h *FreightAgentHandler) Handle(ctx context.Context, userID, chatID, text string) string {
	if text == "" {
		return ""
	}
	text = normalizeText(text)

	log.Printf("[agent] user=%s chat=%s text=%q", userID, chatID, text)

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
		log.Printf("[agent] create task error: %v", err)
	}
	defer func() {
		h.taskStore.Complete(ctx, taskID, taskStatus, time.Since(taskStart).Milliseconds())
	}()

	// ① 意图预判断：输入是否运价相关
	isFreight := isLikelyFreightInput(text)
	intentStatus := store.StepStatusSuccess
	intentOutput := "pass"
	if !isFreight {
		intentStatus = store.StepStatusSkipped
		intentOutput = "reject"
	}
	h.taskStore.AddStep(ctx, &store.TaskStepRecord{
		TaskID: taskID,
		Step:   store.StepIntentCheck,
		Status: intentStatus,
		Input:  store.TruncateText(text, 200),
		Output: intentOutput,
	})
	if !isFreight {
		taskStatus = store.TaskStatusRejected
		log.Printf("[agent] non-freight input, skip parsing")
		return "我是运价分析机器人，可以帮您从运价报价文本中提取结构化数据。\n\n请发送包含以下信息的运价文本：\n• 船公司名称（如万海、MSC、COSCO）\n• 起运港 / 目的港\n• 箱型价格（20GP / 40GP / 40HC）\n• 有效期"
	}

	// ② LLM 运价提取
	llmStart := time.Now()
	rates, err := h.llmService.ParseQuoteInput(ctx, text)
	llmMs := time.Since(llmStart).Milliseconds()
	log.Printf("[agent] ParseQuoteInput elapsed=%s", time.Since(llmStart).Round(time.Millisecond))
	if err != nil {
		taskStatus = store.TaskStatusFailed
		h.taskStore.AddStep(ctx, &store.TaskStepRecord{
			TaskID:     taskID,
			Step:       store.StepLLMParse,
			Status:     store.StepStatusFailed,
			Input:      store.TruncateText(text, 500),
			Error:      err.Error(),
			DurationMS: store.Int64Ptr(llmMs),
		})
		log.Printf("[agent] parse quote error: %v", err)
		return fmt.Sprintf("运价解析失败：%v\n\n请检查格式后重试。", err)
	}
	h.taskStore.AddStep(ctx, &store.TaskStepRecord{
		TaskID:     taskID,
		Step:       store.StepLLMParse,
		Status:     store.StepStatusSuccess,
		Input:      store.TruncateText(text, 500),
		Output:     fmt.Sprintf("解析到 %d 条运价", len(rates)),
		DurationMS: store.Int64Ptr(llmMs),
	})
	if len(rates) == 0 {
		return "未能从您的输入中提取到运价信息。\n\n请提供包含起运港、目的港、运价的运价文本。"
	}

	// ③ 字段校验
	valid, invalid := filterValidRates(rates)
	log.Printf("[agent] rates total=%d valid=%d invalid=%d", len(rates), len(valid), len(invalid))
	h.taskStore.AddStep(ctx, &store.TaskStepRecord{
		TaskID: taskID,
		Step:   store.StepValidate,
		Status: store.StepStatusSuccess,
		Input:  fmt.Sprintf("解析到 %d 条", len(rates)),
		Output: fmt.Sprintf("有效 %d 条，无效 %d 条", len(valid), len(invalid)),
	})
	if len(valid) == 0 {
		return buildInvalidOnlyReply(invalid)
	}

	// ④ 保存有效运价
	saveStart := time.Now()
	saveErr := h.rateStore.BatchSave(ctx, valid, userID, chatID)
	saveMs := time.Since(saveStart).Milliseconds()
	if saveErr != nil {
		taskStatus = store.TaskStatusFailed
		h.taskStore.AddStep(ctx, &store.TaskStepRecord{
			TaskID:     taskID,
			Step:       store.StepDBSave,
			Status:     store.StepStatusFailed,
			Input:      fmt.Sprintf("%d 条有效运价", len(valid)),
			Error:      saveErr.Error(),
			DurationMS: store.Int64Ptr(saveMs),
		})
		log.Printf("[agent] save rates error: %v", saveErr)
		return fmt.Sprintf("⚠️ 运价保存失败：%v", saveErr)
	}
	h.taskStore.AddStep(ctx, &store.TaskStepRecord{
		TaskID:     taskID,
		Step:       store.StepDBSave,
		Status:     store.StepStatusSuccess,
		Input:      fmt.Sprintf("%d 条有效运价", len(valid)),
		Output:     fmt.Sprintf("已保存 %d 条", len(valid)),
		DurationMS: store.Int64Ptr(saveMs),
	})

	return buildRateTableReply(valid, invalid)
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

// buildRateTableReply 构建运价列表回复，按船公司+起运港分组，每组输出一个 Markdown 表格。
// 列动态根据实际提取到的字段决定，没有内容的列不展示。
func buildRateTableReply(rates []llm.FreightRate, invalid []llm.FreightRate) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✅ 已保存 %d 条运价\n", len(rates)))

	// 按 Carrier+POL 分组，保留插入顺序
	type groupKey struct{ Carrier, POL string }
	var order []groupKey
	groups := map[groupKey][]llm.FreightRate{}
	for _, r := range rates {
		k := groupKey{r.Carrier, r.POL}
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

		// 组标题：Carrier | POL(POLCode)
		carrier := key.Carrier
		if carrier == "" {
			carrier = "未知船公司"
		}
		pol := key.POL
		if pol == "" {
			pol = "未知起运港"
		}
		if polCode := group[0].POLCode; polCode != "" {
			pol = pol + "(" + polCode + ")"
		}
		sb.WriteString(fmt.Sprintf("\n**%s | %s**\n", carrier, pol))

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

		// 表头
		sb.WriteString("| 目的港")
		for _, col := range activeCols {
			sb.WriteString(" | " + col.header)
		}
		sb.WriteString(" |\n")

		// 分隔行
		sb.WriteString("|---")
		for range activeCols {
			sb.WriteString("|---")
		}
		sb.WriteString("|\n")

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
			sb.WriteString(" |\n")
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
		case r.ValidityStartTime == "" && r.ValidityEndTime == "":
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
