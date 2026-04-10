package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"freight-agent-wechat/internal/llm"
	"freight-agent-wechat/pkg/config"
	"freight-agent-wechat/pkg/logger"
)

// carrierMapping 承运人名称到代码的映射
var carrierMapping = map[string]string{
	"马士基": "MAERSK", "MAERSK": "MAERSK", "MSK": "MAERSK",
	"中远": "COSCO", "COSCO": "COSCO", "中远海运": "COSCO",
	"地中海": "MSC", "MSC": "MSC",
	"达飞": "CMA", "CMA CGM": "CMA", "CMA": "CMA",
	"长荣": "EMC", "EVERGREEN": "EMC", "EMC": "EMC",
	"海丰": "SITC", "SITC": "SITC",
	"万海": "WHL", "WANHAI": "WHL", "WHL": "WHL",
	"阳明": "YML", "YANGMING": "YML", "YML": "YML",
	"ONE": "ONE",
	"HMM": "HMM",
	"ZIM": "ZIM", "以星": "ZIM",
	"PIL": "PIL", "太平": "PIL",
	"HAPAG": "HLCU", "HAPAG-LLOYD": "HLCU", "HLCU": "HLCU", "赫伯罗特": "HLCU",
	"OOCL": "OOCL", "东方海外": "OOCL",
	"ANL": "ANL",
	"APL": "APL", "美国总统": "APL",
	"KMTC": "KMTC", "高丽": "KMTC",
}

// TokenManager 管理第三方 API 的 OAuth Token
type TokenManager struct {
	mu         sync.RWMutex
	token      string
	expiresAt  time.Time
	httpClient *http.Client
	cfg        *config.PushConfig
}

// NewTokenManager 创建 Token 管理器
func NewTokenManager(cfg *config.PushConfig) *TokenManager {
	return &TokenManager{
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		cfg: cfg,
	}
}

// GetToken 获取有效 token,过期前 10 秒自动刷新
func (tm *TokenManager) GetToken() (string, error) {
	tm.mu.RLock()
	if tm.token != "" && time.Now().Add(10*time.Second).Before(tm.expiresAt) {
		token := tm.token
		tm.mu.RUnlock()
		return token, nil
	}
	tm.mu.RUnlock()

	// 需要刷新 token
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 双重检查
	if tm.token != "" && time.Now().Add(10*time.Second).Before(tm.expiresAt) {
		return tm.token, nil
	}

	if err := tm.refreshToken(); err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}

	return tm.token, nil
}

// refreshToken 调用 getToken API 获取新 token
func (tm *TokenManager) refreshToken() error {
	// 使用 GET 请求,参数作为 query string
	url := fmt.Sprintf("%s/channel/outer/geekyumOauth/getToken?client_id=%s&client_secret=%s&validity_time=%d",
		tm.cfg.BaseURL, tm.cfg.ClientID, tm.cfg.ClientSecret, tm.cfg.TokenTTL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := tm.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		RegisterTime string `json:"registerTime"`
		InvalidTime  string `json:"invalidTime"`
		AccessToken  string `json:"accessToken"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("unmarshal response: %w, body: %s", err, string(body))
	}

	if result.AccessToken == "" {
		return fmt.Errorf("api error: empty accessToken, body: %s", string(body))
	}

	tm.token = result.AccessToken

	// 解析过期时间
	if result.InvalidTime != "" {
		if expireTime, err := time.Parse("2006-01-02 15:04:05", result.InvalidTime); err == nil {
			tm.expiresAt = expireTime
		} else {
			// 解析失败则使用默认 TTL
			tm.expiresAt = time.Now().Add(time.Duration(tm.cfg.TokenTTL) * time.Millisecond)
		}
	} else {
		// 默认使用配置的 TTL
		tm.expiresAt = time.Now().Add(time.Duration(tm.cfg.TokenTTL) * time.Millisecond)
	}

	log.Printf("[push] token refreshed, expires at %s, length=%d", tm.expiresAt.Format(time.RFC3339), len(tm.token))
	return nil
}

// PushService 推送服务
type PushService struct {
	tokenMgr   *TokenManager
	httpClient *http.Client
	cfg        *config.PushConfig
	appLogger  *logger.Logger // 应用日志
}

// NewPushService 创建推送服务
func NewPushService(cfg *config.PushConfig, appLogger *logger.Logger) *PushService {
	return &PushService{
		tokenMgr: NewTokenManager(cfg),
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		cfg:       cfg,
		appLogger: appLogger,
	}
}

// IsEnabled 返回是否启用推送
func (ps *PushService) IsEnabled() bool {
	return ps.cfg.Enabled
}

// PushResult 推送结果
type PushResult struct {
	Total          int           // 总记录数
	Success        int           // 推送成功数
	Failed         int           // 推送失败数
	Skipped        int           // 跳过数(缺少必填字段)
	SuccessIndices []int         // 成功的记录索引
	FailedIndices  []int         // 失败的记录索引
	Failures       []PushFailure // 失败详情
	RawAPIRequest  string        // 发送给第三方API的原始请求参数 JSON
	RawAPIResponse string        // 第三方API返回的原始JSON响应
}

// PushFailure 推送失败详情
type PushFailure struct {
	Index     int    // 原始索引
	Error     string // 错误信息
	Retryable bool   // 是否可重试
}

// PushRates 批量推送运价数据
func (ps *PushService) PushRates(ctx context.Context, rates []llm.FreightRate) PushResult {
	start := time.Now()
	batchID := fmt.Sprintf("push_%d", time.Now().UnixMilli())

	result := PushResult{
		Total: len(rates),
	}

	if len(rates) == 0 {
		return result
	}

	// 转换为推送 payload
	payload, skipReasons := ps.convertToPushPayload(rates)
	result.Skipped = len(skipReasons)
	for _, reason := range skipReasons {
		if ps.appLogger != nil {
			ps.appLogger.Warn("[push] skip: %s", reason)
		}
	}

	if len(payload.List) == 0 {
		result.Failed = result.Total - result.Skipped
		if ps.appLogger != nil {
			ps.appLogger.Error("[push] batch %s failed: no valid rates to push", batchID)
		}
		return result
	}

	// 保存推送请求参数 JSON
	if payloadJSON, err := json.Marshal(payload); err == nil {
		result.RawAPIRequest = string(payloadJSON)
	}

	// 推送数据
	resp, rawAPIResponse, err := ps.pushWithRetry(ctx, payload)
	log.Printf("[push] batch %s completed in %dms", batchID, time.Since(start).Milliseconds())

	// 保存原始API响应
	result.RawAPIResponse = rawAPIResponse

	if err != nil {
		log.Printf("[push] push error: %v", err)
		if ps.appLogger != nil {
			ps.appLogger.Error("[push] push error: %v", err)
		}
		result.Failed = len(payload.List)

		// 设置失败索引（全部失败）
		for i := range payload.List {
			result.FailedIndices = append(result.FailedIndices, i)
			result.Failures = append(result.Failures, PushFailure{
				Index:     i,
				Error:     err.Error(),
				Retryable: classifyError(err.Error()) == ErrorNetwork,
			})
		}

		// 记录 ERROR 日志
		if ps.appLogger != nil {
			ps.appLogger.Error("[push] batch %s failed: %v", batchID, err)
		}
		return result
	}

	// 解析响应
	if resp.Success {
		result.Success = len(payload.List)
		// 设置所有索引为成功
		for i := 0; i < len(payload.List); i++ {
			result.SuccessIndices = append(result.SuccessIndices, i)
		}
		log.Printf("[push] successfully pushed %d rates", result.Success)
		if ps.appLogger != nil {
			ps.appLogger.Info("[push] successfully pushed %d rates", result.Success)
		}

		// 记录 INFO 日志
		if ps.appLogger != nil {
			ps.appLogger.Info("[push] batch %s success: %d rates", batchID, result.Success)
		}
	} else {
		// 解析错误消息，判断哪些记录失败
		// 错误消息格式："批量新增异常 第1条数据：系统暂无当前承运人..."
		failedIndices := parseFailedIndices(resp.Msg, len(payload.List))

		result.Failed = len(failedIndices)
		result.Success = len(payload.List) - result.Failed
		result.FailedIndices = failedIndices

		// 构建成功索引列表
		for i := 0; i < len(payload.List); i++ {
			if !containsInt(failedIndices, i) {
				result.SuccessIndices = append(result.SuccessIndices, i)
			}
		}

		// 记录失败详情
		errMsg := fmt.Sprintf("api error: code=%v, msg=%s", resp.Code, resp.Msg)
		for _, idx := range failedIndices {
			result.Failures = append(result.Failures, PushFailure{
				Index:     idx,
				Error:     errMsg,
				Retryable: classifyError(resp.Msg) == ErrorNetwork,
			})
		}

		log.Printf("[push] push failed: %s", resp.Msg)
		if ps.appLogger != nil {
			ps.appLogger.Error("[push] push failed: %s", resp.Msg)
		}

		// 记录 ERROR 日志
		if ps.appLogger != nil {
			ps.appLogger.Error("[push] batch %s partial failed: success=%d failed=%d, msg=%s",
				batchID, result.Success, result.Failed, resp.Msg)
		}
	}

	return result
}

// PushPayload 推送到第三方的数据结构
type PushPayload struct {
	List []RateItem `json:"list"`
}

// RateItem 单条运价数据
type RateItem struct {
	ProductSource      string                    `json:"productSource"`
	CarrierCode        string                    `json:"carrierCode"`
	PorCode            string                    `json:"porCode"`
	PolCode            string                    `json:"polCode"`
	PodCode            string                    `json:"podCode"`
	PdlCode            string                    `json:"pdlCode"`
	ContainerPrice     map[string]ContainerPrice `json:"containerPrice"`
	Etd                int64                     `json:"etd"`
	Eta                int64                     `json:"eta,omitempty"` // 新增：ETA 时间
	IsVia              int                       `json:"isVia"`
	TransitPort        []string                  `json:"transitPort"`
	VesselName         string                    `json:"vesselName,omitempty"`
	Voyage             string                    `json:"voyage,omitempty"`
	RouteCode          string                    `json:"routeCode,omitempty"` // 新增：航线代码
	TransportDay       int                       `json:"transportDay,omitempty"`
	FreeTime           string                    `json:"freeTime,omitempty"`
	CutOffDate         string                    `json:"cutOffDate,omitempty"`
	ValidStartDate     int64                     `json:"validStartDate"`
	ValidEndDate       int64                     `json:"validEndDate"`
	ProductName        string                    `json:"productName,omitempty"`
	WeightLimitRemark  string                    `json:"weightLimitRemark,omitempty"`
	Remark             string                    `json:"remark,omitempty"`
	ContactInfoID      int64                     `json:"contactInfoId,omitempty"` // 新增：联系人信息 ID
	PromotionStartDate int64                     `json:"promotionStartDate"`
}

// ContainerPrice 箱型价格
type ContainerPrice struct {
	ContainerType string  `json:"containerType"`
	Currency      string  `json:"currency"`
	Price         float64 `json:"price"`
}

// PushResponse 推送响应
type PushResponse struct {
	Success bool        `json:"success"`
	Code    interface{} `json:"code"` // 可能是 string 或 int
	Msg     string      `json:"msg"`
	Data    interface{} `json:"data"`
}

// convertToPushPayload 将运价数据转换为第三方接口格式
func (ps *PushService) convertToPushPayload(rates []llm.FreightRate) (*PushPayload, []string) {
	payload := &PushPayload{}
	var skipReasons []string

	for i, r := range rates {
		// 必填字段校验
		var missing []string
		if r.Agent == "" {
			missing = append(missing, "Agent")
		}

		carrierCode := ps.mapCarrierCode(r.Carrier)
		if carrierCode == "" {
			missing = append(missing, "Carrier(无法映射)")
		}

		if r.POLCode == "" {
			missing = append(missing, "POLCode")
		}
		if r.PODCode == "" {
			missing = append(missing, "PODCode")
		}

		etdMs := ps.parseDateToMs(r.ETD)
		if etdMs == 0 {
			missing = append(missing, "ETD")
		}

		validStartMs := ps.parseDateToMs(r.ValidityStartTime)
		validEndMs := ps.parseDateToMs(r.ValidityEndTime)
		if validStartMs == 0 || validEndMs == 0 {
			missing = append(missing, "ValidityTime")
		}

		// 检查是否有至少一个箱型价格
		containerPrice := ps.buildContainerPrice(r)
		if len(containerPrice) == 0 {
			missing = append(missing, "ContainerPrice")
		}

		if len(missing) > 0 {
			skipReasons = append(skipReasons, fmt.Sprintf("index %d: missing %v", i, missing))
			continue
		}

		// 构建数据项
		item := RateItem{
			ProductSource:      r.Agent,
			CarrierCode:        carrierCode,
			PorCode:            r.POLCode,
			PolCode:            r.POLCode,
			PodCode:            r.PODCode,
			PdlCode:            r.PODCode,
			ContainerPrice:     containerPrice,
			Etd:                etdMs,
			IsVia:              ps.checkIsVia(r.Remark),
			TransitPort:        []string{},
			ValidStartDate:     validStartMs,
			ValidEndDate:       validEndMs,
			PromotionStartDate: time.Now().UnixMilli(),
		}

		// 可选字段
		if r.Vessel != "" {
			item.VesselName = r.Vessel
		}
		if r.Voyage != "" {
			item.Voyage = r.Voyage
		}
		if r.POLPODTT != "" {
			// 尝试解析航程天数
			if days := ps.parseInt(r.POLPODTT); days > 0 {
				item.TransportDay = days
			}
		}
		if r.DND != "" {
			item.FreeTime = r.DND
		}
		if r.CutOffTime != "" {
			item.CutOffDate = r.CutOffTime
		}
		if r.WeightLimit != "" {
			item.WeightLimitRemark = r.WeightLimit
		}

		// 构建备注
		var remark string
		remark = r.Agent

		if r.Remark != "" {
			remark = remark + "-" + r.Remark
		}
		if r.Commodity != "" {
			item.ProductName = r.Commodity
		}

		payload.List = append(payload.List, item)
	}

	return payload, skipReasons
}

// buildContainerPrice 构建箱型价格
func (ps *PushService) buildContainerPrice(r llm.FreightRate) map[string]ContainerPrice {
	price := make(map[string]ContainerPrice)

	if r.F20GP != "" {
		if p := ps.parseFloat(r.F20GP); p > 0 {
			price["20GP"] = ContainerPrice{
				ContainerType: "20GP",
				Currency:      "USD",
				Price:         p,
			}
		}
	}
	if r.F40GP != "" {
		if p := ps.parseFloat(r.F40GP); p > 0 {
			price["40GP"] = ContainerPrice{
				ContainerType: "40GP",
				Currency:      "USD",
				Price:         p,
			}
		}
	}
	if r.F40HC != "" {
		if p := ps.parseFloat(r.F40HC); p > 0 {
			price["40HC"] = ContainerPrice{
				ContainerType: "40HC",
				Currency:      "USD",
				Price:         p,
			}
		}
	}

	return price
}

// pushWithRetry 带重试的推送
// 返回：PushResponse, 原始JSON响应, error
func (ps *PushService) pushWithRetry(ctx context.Context, payload *PushPayload) (*PushResponse, string, error) {
	var lastErr error

	for attempt := 0; attempt <= ps.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避: 1s, 2s, 4s
			wait := time.Duration(1<<uint(attempt-1)) * time.Second
			log.Printf("[push] retry attempt %d/%d after %v", attempt, ps.cfg.MaxRetries, wait)
			if ps.appLogger != nil {
				ps.appLogger.Warn("[push] retry attempt %d/%d after %v", attempt, ps.cfg.MaxRetries, wait)
			}
			time.Sleep(wait)
		}

		resp, rawJSON, err := ps.doPush(ctx, payload)
		if err == nil {
			return resp, rawJSON, nil
		}

		lastErr = err
		errType := classifyError(err.Error())

		// 数据错误不重试
		if errType == ErrorData {
			return nil, rawJSON, err
		}

		// 认证错误,刷新 token 后重试
		if errType == ErrorAuth {
			log.Printf("[push] auth error, refreshing token")
			if ps.appLogger != nil {
				ps.appLogger.Warn("[push] auth error, refreshing token")
			}
			if refreshErr := ps.tokenMgr.refreshToken(); refreshErr != nil {
				log.Printf("[push] token refresh failed: %v", refreshErr)
				if ps.appLogger != nil {
					ps.appLogger.Error("[push] token refresh failed: %v", refreshErr)
				}
			}
		}
	}

	return nil, "", fmt.Errorf("push failed after %d retries: %w", ps.cfg.MaxRetries, lastErr)
}

// doPush 执行单次推送
// 返回：PushResponse, 原始JSON响应, error
func (ps *PushService) doPush(ctx context.Context, payload *PushPayload) (*PushResponse, string, error) {
	token, err := ps.tokenMgr.GetToken()
	if err != nil {
		return nil, "", fmt.Errorf("get token: %w", err)
	}

	if ps.appLogger != nil {
		ps.appLogger.Info("[push] using token (length=%d) for request", len(token))
	}

	// Token 通过 query 参数 access_token 传递
	url := fmt.Sprintf("%s/channel/outer/rateLink/preferredProductBatchCreate?access_token=%s",
		ps.cfg.BaseURL, token)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ps.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, string(body), fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var result PushResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, string(body), fmt.Errorf("unmarshal response: %w, body: %s", err, string(body))
	}

	if !result.Success {
		return &result, string(body), fmt.Errorf("api error: code=%v, msg=%s", result.Code, result.Msg)
	}

	return &result, string(body), nil
}

// mapCarrierCode 映射承运人代码
func (ps *PushService) mapCarrierCode(carrier string) string {
	if carrier == "" {
		return ""
	}

	// 直接匹配
	if code, ok := carrierMapping[carrier]; ok {
		return code
	}

	// 大写匹配
	upper := strings.ToUpper(carrier)
	if code, ok := carrierMapping[upper]; ok {
		return code
	}

	// 模糊匹配
	for key, code := range carrierMapping {
		if strings.Contains(strings.ToUpper(carrier), strings.ToUpper(key)) {
			return code
		}
	}

	return ""
}

// parseDateToMs 解析日期字符串为毫秒时间戳
// 支持格式: yyyy/MM/dd, yyyy-MM-dd
func (ps *PushService) parseDateToMs(dateStr string) int64 {
	if dateStr == "" {
		return 0
	}

	// 尝试多种格式
	formats := []string{
		"2006/01/02",
		"2006-01-02",
		"2006/1/2",
		"2006-1-2",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t.UnixMilli()
		}
	}

	log.Printf("[push] failed to parse date: %s", dateStr)
	if ps.appLogger != nil {
		ps.appLogger.Warn("[push] failed to parse date: %s", dateStr)
	}
	return 0
}

// checkIsVia 检查是否中转
func (ps *PushService) checkIsVia(remark string) int {
	if strings.Contains(strings.ToLower(remark), "via") {
		return 1
	}
	return 0
}

// parseFloat 解析浮点数
func (ps *PushService) parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// parseInt 解析整数
func (ps *PushService) parseInt(s string) int {
	var i int
	fmt.Sscanf(s, "%d", &i)
	return i
}

// PushErrorType 错误类型
type PushErrorType int

const (
	ErrorNetwork PushErrorType = iota // 网络错误,可重试
	ErrorData                         // 数据异常,不可重试
	ErrorAuth                         // 认证错误,需刷新 token
)

// classifyError 根据错误信息判断错误类型
func classifyError(errMsg string) PushErrorType {
	errLower := strings.ToLower(errMsg)

	// 认证错误
	if strings.Contains(errLower, "token") ||
		strings.Contains(errLower, "auth") ||
		strings.Contains(errLower, "401") ||
		strings.Contains(errLower, "unauthorized") {
		return ErrorAuth
	}

	// 数据错误(不可重试)
	if strings.Contains(errLower, "承运人") ||
		strings.Contains(errLower, "系统暂无") ||
		strings.Contains(errLower, "不存在") ||
		strings.Contains(errLower, "无效") ||
		strings.Contains(errLower, "格式错误") {
		return ErrorData
	}

	// 默认视为网络错误(可重试)
	return ErrorNetwork
}

// parseFailedIndices 从错误消息中解析失败的记录索引
// 错误消息格式："批量新增异常 第1条数据：...；第3条数据：..."
func parseFailedIndices(errMsg string, total int) []int {
	var failedIndices []int

	// 正则匹配 "第X条数据"
	re := regexp.MustCompile(`第(\d+)条数据`)
	matches := re.FindAllStringSubmatch(errMsg, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			if idx, err := strconv.Atoi(match[1]); err == nil {
				// API 返回的是 1-based 索引，转换为 0-based
				if idx > 0 && idx <= total {
					failedIndices = append(failedIndices, idx-1)
				}
			}
		}
	}

	// 如果没有解析到具体索引，默认全部失败
	if len(failedIndices) == 0 {
		for i := 0; i < total; i++ {
			failedIndices = append(failedIndices, i)
		}
	}

	return failedIndices
}

// containsInt 检查切片中是否包含指定整数
func containsInt(slice []int, item int) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
