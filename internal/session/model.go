package session

import (
	"fmt"
	"time"

	"freight-agent-wechat/internal/quote"
)

// QuoteSession 报价单会话
type QuoteSession struct {
	SessionID    string               `json:"session_id"`
	UserID       string               `json:"user_id"`
	GroupID      string               `json:"group_id"`
	CurrentQuote *quote.QuoteData     `json:"current_quote"`
	History      []quote.QuoteVersion `json:"history"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
}

// SessionKey 生成会话Key
func SessionKey(groupID, userID string) string {
	return fmt.Sprintf("%s:%s", groupID, userID)
}

// NewQuoteSession 创建新会话
func NewQuoteSession(groupID, userID string) *QuoteSession {
	now := time.Now()
	return &QuoteSession{
		SessionID:    SessionKey(groupID, userID),
		UserID:       userID,
		GroupID:      groupID,
		CurrentQuote: nil,
		History:      make([]quote.QuoteVersion, 0),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// SetQuote 设置当前报价单并保存历史
func (s *QuoteSession) SetQuote(quoteData *quote.QuoteData, operation string) {
	if s.CurrentQuote != nil {
		// 保存当前版本到历史
		version := quote.QuoteVersion{
			VersionID: fmt.Sprintf("v%d", len(s.History)+1),
			QuoteData: *s.CurrentQuote.Clone(),
			Operation: operation,
			Timestamp: time.Now(),
		}
		s.History = append(s.History, version)
	}
	s.CurrentQuote = quoteData
	s.UpdatedAt = time.Now()
}

// Rollback 回滚到指定版本
func (s *QuoteSession) Rollback(versionIndex int) error {
	if versionIndex < 0 || versionIndex >= len(s.History) {
		return fmt.Errorf("invalid version index: %d", versionIndex)
	}

	// 恢复指定版本
	s.CurrentQuote = s.History[versionIndex].QuoteData.Clone()
	s.UpdatedAt = time.Now()
	return nil
}

// GetHistorySummary 获取历史版本摘要
func (s *QuoteSession) GetHistorySummary() []map[string]interface{} {
	summary := make([]map[string]interface{}, 0, len(s.History))
	for i, v := range s.History {
		summary = append(summary, map[string]interface{}{
			"index":     i,
			"version":   v.VersionID,
			"operation": v.Operation,
			"timestamp": v.Timestamp.Format("01-02 15:04"),
		})
	}
	return summary
}

// IsExpired 检查会话是否过期
func (s *QuoteSession) IsExpired(ttl time.Duration) bool {
	return time.Since(s.UpdatedAt) > ttl
}
