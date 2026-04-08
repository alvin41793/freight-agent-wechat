package session

import (
	"context"
	"fmt"
	"time"

	"freight-agent-wechat/internal/quote"
)

const (
	// DefaultSessionTTL 默认会话过期时间（30分钟）
	DefaultSessionTTL = 30 * time.Minute
)

// Manager 会话管理器
type Manager struct {
	store Store
	ttl   time.Duration
}

// NewManager 创建会话管理器
func NewManager(store Store) *Manager {
	return &Manager{
		store: store,
		ttl:   DefaultSessionTTL,
	}
}

// NewManagerWithTTL 创建带自定义 TTL 的会话管理器
func NewManagerWithTTL(store Store, ttl time.Duration) *Manager {
	return &Manager{
		store: store,
		ttl:   ttl,
	}
}

// GetOrCreateSession 获取或创建会话
func (m *Manager) GetOrCreateSession(ctx context.Context, groupID, userID string) (*QuoteSession, error) {
	sessionKey := SessionKey(groupID, userID)

	// 尝试获取现有会话
	session, err := m.store.Get(ctx, sessionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// 如果会话不存在或已过期，创建新会话
	if session == nil || session.IsExpired(m.ttl) {
		session = NewQuoteSession(groupID, userID)
		if err := m.store.Set(ctx, session, m.ttl); err != nil {
			return nil, fmt.Errorf("failed to save new session: %w", err)
		}
	}

	return session, nil
}

// GetSession 获取会话（不自动创建）
func (m *Manager) GetSession(ctx context.Context, groupID, userID string) (*QuoteSession, error) {
	sessionKey := SessionKey(groupID, userID)
	session, err := m.store.Get(ctx, sessionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if session != nil && session.IsExpired(m.ttl) {
		// 会话已过期，删除它
		_ = m.store.Delete(ctx, sessionKey)
		return nil, nil
	}

	return session, nil
}

// SaveSession 保存会话
func (m *Manager) SaveSession(ctx context.Context, session *QuoteSession) error {
	if err := m.store.Set(ctx, session, m.ttl); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}
	return nil
}

// UpdateQuote 更新会话中的报价单
func (m *Manager) UpdateQuote(ctx context.Context, groupID, userID string, quoteData *quote.QuoteData, operation string) error {
	session, err := m.GetOrCreateSession(ctx, groupID, userID)
	if err != nil {
		return err
	}

	session.SetQuote(quoteData, operation)
	return m.SaveSession(ctx, session)
}

// GetCurrentQuote 获取当前报价单
func (m *Manager) GetCurrentQuote(ctx context.Context, groupID, userID string) (*quote.QuoteData, error) {
	session, err := m.GetSession(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}

	if session == nil {
		return nil, nil
	}

	return session.CurrentQuote, nil
}

// HasActiveQuote 检查是否有活跃的报价单
func (m *Manager) HasActiveQuote(ctx context.Context, groupID, userID string) bool {
	quote, _ := m.GetCurrentQuote(ctx, groupID, userID)
	return quote != nil
}

// RollbackQuote 回滚报价单到指定版本
func (m *Manager) RollbackQuote(ctx context.Context, groupID, userID string, versionIndex int) (*quote.QuoteData, error) {
	session, err := m.GetSession(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}

	if session == nil {
		return nil, fmt.Errorf("no active session found")
	}

	if err := session.Rollback(versionIndex); err != nil {
		return nil, err
	}

	if err := m.SaveSession(ctx, session); err != nil {
		return nil, err
	}

	return session.CurrentQuote, nil
}

// GetHistory 获取报价历史
func (m *Manager) GetHistory(ctx context.Context, groupID, userID string) ([]map[string]interface{}, error) {
	session, err := m.GetSession(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}

	if session == nil {
		return nil, nil
	}

	return session.GetHistorySummary(), nil
}

// ClearSession 清空会话
func (m *Manager) ClearSession(ctx context.Context, groupID, userID string) error {
	sessionKey := SessionKey(groupID, userID)
	return m.store.Delete(ctx, sessionKey)
}
