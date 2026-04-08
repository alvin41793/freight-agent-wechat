package quote

import (
	"time"
)

// RouteInfo 航线信息
type RouteInfo struct {
	POL string `json:"pol"` // 起运港 (Port of Loading)
	POD string `json:"pod"` // 目的港 (Port of Discharge)
}

// QuoteItem 报价项（集装箱类型和价格）
type QuoteItem struct {
	ContainerType string  `json:"container_type"` // 20GP, 40HQ, 45HQ, etc.
	Price         float64 `json:"price"`
	Currency      string  `json:"currency"` // USD, CNY
}

// Surcharge 附加费
type Surcharge struct {
	Name  string  `json:"name"` // 附加费名称
	Price float64 `json:"price"`
	Unit  string  `json:"unit"` // per_container, lump_sum
}

// QuoteData 报价单数据
type QuoteData struct {
	QuoteID    string      `json:"quote_id"`
	Route      RouteInfo   `json:"route"`
	Items      []QuoteItem `json:"items"`
	Surcharges []Surcharge `json:"surcharges"`
	ValidUntil *time.Time  `json:"valid_until,omitempty"`
	Remarks    string      `json:"remarks"`
	Currency   string      `json:"currency"` // 默认货币
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// QuoteVersion 报价单版本
type QuoteVersion struct {
	VersionID string    `json:"version_id"`
	QuoteData QuoteData `json:"quote_data"`
	Operation string    `json:"operation"` // create, update, delete
	Timestamp time.Time `json:"timestamp"`
}

// NewQuoteData 创建新的报价单
func NewQuoteData() *QuoteData {
	now := time.Now()
	return &QuoteData{
		QuoteID:    generateQuoteID(),
		Items:      make([]QuoteItem, 0),
		Surcharges: make([]Surcharge, 0),
		Currency:   "USD",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// generateQuoteID 生成报价单ID
func generateQuoteID() string {
	return "Q" + time.Now().Format("20060102150405") + randomString(4)
}

// randomString 生成随机字符串
func randomString(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(result)
}

// CalculateTotal 计算总价
func (q *QuoteData) CalculateTotal() float64 {
	var total float64
	for _, item := range q.Items {
		total += item.Price
	}
	for _, surcharge := range q.Surcharges {
		if surcharge.Unit == "per_container" {
			total += surcharge.Price * float64(len(q.Items))
		} else {
			total += surcharge.Price
		}
	}
	return total
}

// Clone 深拷贝报价单
func (q *QuoteData) Clone() *QuoteData {
	cloned := &QuoteData{
		QuoteID:    q.QuoteID,
		Route:      q.Route,
		Items:      make([]QuoteItem, len(q.Items)),
		Surcharges: make([]Surcharge, len(q.Surcharges)),
		ValidUntil: q.ValidUntil,
		Remarks:    q.Remarks,
		Currency:   q.Currency,
		CreatedAt:  q.CreatedAt,
		UpdatedAt:  q.UpdatedAt,
	}
	copy(cloned.Items, q.Items)
	copy(cloned.Surcharges, q.Surcharges)
	return cloned
}
