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
	client *Client
	log    *logger.Logger // 可为 nil（pro 模式）
}

// NewService 创建 LLM 服务（无调试日志）
func NewService(client *Client) *Service {
	return &Service{client: client}
}

// NewServiceWithLogger 创建 LLM 服务（debug 模式下记录输入输出到文件）
func NewServiceWithLogger(client *Client, log *logger.Logger) *Service {
	return &Service{client: client, log: log}
}

const extractSystemPrompt = `你是海运运价结构化解析专家。

# 任务
从输入的非结构化文本中，提取"可确定"的海运运价信息，并输出标准 JSON 数组。

⚠️ 核心原则：
只提取"明确关联"的信息，禁止猜测、禁止跨段拼接。

--------------------------------
# 一、输出格式要求
1. 仅输出一个 JSON 数组（即使只有一条）。
2. 每条记录必须满足：
   - 至少包含：POL + POD + 运价（F20GP / F40GP / F40HC 任一）
   - 信息必须来源于同一语义块
3. 不要输出 null 字段，缺失字段直接省略
4. 禁止输出任何解释、说明或额外文字

--------------------------------
# 二、字段定义（严格使用以下 Key）

- Carrier（承运人标准代码，如 MSK、ONE、EMC、ZIM、WHL、CMA、HMM、MSC）
- POL（起运港，单个港口英文名称）
- POLCode（起运港五字码，如 CNNGB）
- POD（目的港，单个港口英文名称）
- PODCode（目的港五字码，如 USHOU）
- F20GP（20GP 价格）
- F40GP（40GP 价格）
- F40HC（40HC 价格）
- ValidityStartTime（进港有效期开始时间，格式 yyyy/MM/dd）
- ValidityEndTime（进港有效期结束时间，格式 yyyy/MM/dd）
- ETD（预计开航日期，格式 yyyy/MM/dd）
- ETA（预计到港日期，格式 yyyy/MM/dd）
- POLPODTT（航程：数字字符串，仅数字）
- Vessel（船名）
- Voyage（航次）
- Cut-off Time（截关日期，格式 yyyy/MM/dd）
- PortClosingTime（集港/结港日期，格式 yyyy/MM/dd）
- Commodity（品名）
- DND（免箱免堆）
- Weight limit（限重）
- remark（备注）

--------------------------------
# 三、关键解析规则（按优先级排序）

## 1️⃣ 信息绑定规则（最高优先级）

只有在以下情况下，才允许把字段放在同一条记录：
- 同一行
- 或同一明显分组（如同一段落内"船期 + 对应港口 + 对应价格"）

❌ 禁止：
- 船期块 + 价格块跨段拼接
- ETD / Vessel 未明确对应价格时输出这些字段

--------------------------------

## 2️⃣ 港口拆分规则

- POD / POL 出现多个（如 HOUSTON/MOBILE）→ 必须拆分为多条记录
- 每条记录只包含一个 POL 和一个 POD

--------------------------------

## 3️⃣ 区域港口映射规则（重要）

以下区域描述，直接映射到具体港口：

| 区域描述 | 映射港口 | 港口代码 |
|----------|----------|----------|
| 美西 / USWC | LOS ANGELES | USLAX |
| 美东 / EC | NEW YORK | USNYC |
| 美湾 / GULF | HOUSTON | USHOU |

👉 处理方式：
- 直接作为 POD 输出
- 正常输出价格
- remark 不需要额外标注

**特殊情况处理：**

| 输入 | 处理方式 |
|------|----------|
| 美西DG | POD = LOS ANGELES，Commodity = "DG" |
| 美东RF | POD = NEW YORK，Commodity = "RF" |
| 美湾NOR | POD = HOUSTON，Commodity = "NOR" |

以下内容属于区域描述，但**不映射到具体港口**，改为写入 remark：
- 仅当价格无明确对应关系时

--------------------------------

## 4️⃣ 内陆点（IPI）处理规则

以下城市视为内陆点：
- CHICAGO / DALLAS / MEMPHIS / KANSAS / DETROIT / MINNEAPOLIS / ST LOUIS 等

👉 处理方式（三选一）：

**情况A：只有内陆点，无海港**
- 示例：Toronto 4150+
- 处理：POD = 该内陆点，PODCode = 对应代码，remark 标注"内陆点"

**情况B：海港 + 内陆点加价（价格未拆分）**
- 示例：LA + IPI CHI +2200
- 处理：POD = 海港，remark 标注"IPI CHI +2200"（价格不拆分）

**情况C：海港 + 内陆点 + 独立价格**
- 示例：洛杉矶转内陆 芝加哥 4550 5050
- 特征：内陆点有独立的20GP/40GP价格
- 处理：POD = 该内陆点，PODCode = 对应代码，正常输出价格
- remark 标注"洛杉矶转内陆"（说明转运路径））

--------------------------------

## 5️⃣ 多价格顺序映射规则

当价格格式为连续数字（如 1775/2215/2215/2715 ,  NOR 2000++）：

| 位置 | 映射字段 |
|------|----------|
| 第1个 | F20GP |
| 第2个 | F40GP |
| 第3个 | F40HC |
| 第4个及以后 | 写入 remark（如"45HQ:2715", "NOR:2000++"） |

⚠️ **特殊格式解析**：
- "2600+40HQ" → 表示 40HQ 价格为 2600，映射到 F40HC = "2600"
- "2500+40HQ" → 同上
- 格式如 "X+箱型" 中的 X 即为该箱型的运价

注意：以下箱型报价放到备注里
- NOR（是非营运冷箱）
- DG（是危险品）
- RF（是冷藏箱）
- OT/FR/TK（是特殊箱型）
- **45HQ / 45HC / 45GP**（非标准箱型，无对应字段）

--------------------------------

## 6️⃣ 多承运人处理规则

当承运人写为多个（如 MSC ZIM / MSC、ZIM）：

👉 必须拆分为多条记录，每条记录对应一个承运人，价格相同

--------------------------------

## 7️⃣ 价格后缀处理规则

| 后缀 | 处理方式 |
|------|----------|
| "++" | remark 标注"++" |
| "+A" | remark 标注"+A（附加费另计）" |
| "含附加费" | remark 标注"已含附加费" |
| "AMS+换单费" | remark 标注"+AMS+换单费" |

--------------------------------

## 8️⃣ 进港有效期规则（强制输出）

识别关键词：
- "进港" / "进港有效期" / "进港价格" / "gate in"

处理方式：
- 起始日期 → ValidityStartTime
- 结束日期 → ValidityEndTime
- 格式：yyyy/MM/dd

示例：
- "4.06-4.12进港价格" → ValidityStartTime: "2026/04/06", ValidityEndTime: "2026/04/12"
- "gate in: Apr 6 - 12" → ValidityStartTime: "2026/04/06", ValidityEndTime: "2026/04/12"

--------------------------------

## 9️⃣ 截关 vs 集港（严格区分）

| 关键词 | 字段 |
|--------|------|
| 截关 / 截单 / 报关截止 | Cut-off Time |
| 集港 / 进港 / gate in / 结港 / 关港 | PortClosingTime |

⚠️ 若只出现"进港"，填写 PortClosingTime，不要填 Cut-off Time

如果两者同时出现，分别独立输出，不允许互相推断。

--------------------------------

## 🔟 收货/广告信息排除规则

识别以下模式，**不提取为运价记录**：
- "按XXX收货" → 这是货代收货成本，不是对外卖价
- "收XXX" / "收货价" / "成本XXX"
- 整段明显是内部指示而非报价（如"按2500收货，IPI..."）

👉 处理方式：忽略该行或该段落，不作为输出记录

⚠️ **注意区分**：
- 卖价/报价 → 提取
- 收货价/成本价 → 不提取

--------------------------------

## 1️⃣1️⃣ 无法确定规则（安全规则）

以下情况必须放弃该字段（不要猜测）：
- 港口与价格无明确对应关系
- 船名与航线未绑定到具体价格
- 价格数字无法确认对应哪个箱型
- 日期格式无法解析
- 仅出现收货价而无明确报价

**宁可少输出，不输出错误数据。**

--------------------------------

# 四、日期处理规则

- 年份固定：2026
- 格式：yyyy/MM/dd
- 输入格式参考：04-10 → 2026/04/10，4/6 → 2026/04/06，Apr 6 → 2026/04/06

--------------------------------

# 五、价格处理规则

- 只保留数字部分
- 必须为字符串类型（如 "3000"，不是 3000）
- 移除货币符号（$、USD 等）

--------------------------------

# 六、港口代码补充规则

对于常见港口，补充标准 UN/LOCODE 五字码：

| 港口 | 代码 |
|------|------|
| NINGBO | CNNGB |
| SHANGHAI | CNSHA |
| SHENZHEN/YANTIAN | CNSZX |
| QINGDAO | CNQDG |
| XIAMEN | CNXMN |
| LOS ANGELES | USLAX |
| LONG BEACH | USLGB |
| OAKLAND | USOAK |
| NEW YORK | USNYC |
| SAVANNAH | USSAV |
| HOUSTON | USHOU |
| MIAMI | USMIA |
| NORFOLK | USNFK |
| CHARLESTON | USCHS |
| JACKSONVILLE | USJAX |
| BALTIMORE | USBAL |
| TAMPA | USTPA |
| MOBILE | USMOB |
| VANCOUVER | CAVAN |
| TORONTO | CATOR |
| MONTREAL | CAMTR |
| CHICAGO | USCHI |

未列出的港口：按常识补充，不确定则不输出 PODCode。

--------------------------------
# 七、输出示例

**示例1：标准报价**
输入：“MSK 宁波出 ETD 04-10 MAERSK SHAMS/614E SAV 2700/3000++
以上为4.06-4.12进港价格”

输出：
[{"Carrier":"MSK","POL":"NINGBO","POLCode":"CNNGB","POD":"SAVANNAH","PODCode":"USSAV","F20GP":"2700","F40GP":"3000","ETD":"2026/04/10","Vessel":"MAERSK SHAMS","Voyage":"614E","ValidityStartTime":"2026/04/06","ValidityEndTime":"2026/04/12","remark":"++"}]`

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

	var rates []FreightRate
	reasoningContent, err := s.client.CompleteWithJSON(ctx, &CompletionRequest{
		SystemPrompt: extractSystemPrompt,
		UserPrompt:   userPrompt,
	}, &rates)

	// debug 日志
	if s.log != nil && s.log.IsDebug() {
		entry := &logger.LLMEntry{
			Time:             time.Now(),
			Function:         "ParseQuoteInput",
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
		s.log.LogLLM(entry)
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

	systemPrompt := fmt.Sprintf(`你是海运运价数据补全助手。用户之前提供了一批运价数据，但部分记录缺少必要字段，现在用户补充了相关信息。

当前待完善的运价记录（JSON格式）：
%s

缺失字段说明：
%s

请根据用户新提供的补充信息，填充上述缺失字段，输出完整的 JSON 数组。规则：
1. 仅修改缺失的字段，其他字段保持不变
2. 日期格式统一为 yyyy/MM/dd，当前年份为 2026 年
3. 价格字段只保留数字
4. 输出完整的 JSON 数组，包含所有记录（包括已完整和待补充的）`, string(pendingJSON), missingDesc)

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
		s.log.LogLLM(entry)
	}

	return rates, err
}
