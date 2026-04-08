package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"freight-agent-wechat/internal/llm"

	"github.com/go-redis/redis/v8"
)

const PendingRatesTTL = 30 * time.Minute

// PendingRates 待补充的运价会话状态
type PendingRates struct {
	Rates         []llm.FreightRate `json:"rates"`
	MissingFields map[int][]string  `json:"missing_fields"`
	CreatedAt     time.Time         `json:"created_at"`
}

// PendingStore Redis 存储，用于暂存待补充的运价
type PendingStore struct {
	rdb *redis.Client
}

// NewPendingStore 创建待补充运价存储
func NewPendingStore(rdb *redis.Client) *PendingStore {
	return &PendingStore{rdb: rdb}
}

// pendingKey 生成 Redis Key
func pendingKey(userID, chatID string) string {
	return fmt.Sprintf("freight:pending:%s:%s", userID, chatID)
}

// Set 保存待补充运价（TTL 30 分钟）
func (s *PendingStore) Set(ctx context.Context, userID, chatID string, pending *PendingRates) error {
	if pending.CreatedAt.IsZero() {
		pending.CreatedAt = time.Now()
	}
	data, err := json.Marshal(pending)
	if err != nil {
		return fmt.Errorf("marshal pending rates: %w", err)
	}
	return s.rdb.Set(ctx, pendingKey(userID, chatID), data, PendingRatesTTL).Err()
}

// Get 获取待补充运价，不存在时返回 nil
func (s *PendingStore) Get(ctx context.Context, userID, chatID string) (*PendingRates, error) {
	data, err := s.rdb.Get(ctx, pendingKey(userID, chatID)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get pending rates: %w", err)
	}
	var pending PendingRates
	if err := json.Unmarshal([]byte(data), &pending); err != nil {
		return nil, fmt.Errorf("unmarshal pending rates: %w", err)
	}
	return &pending, nil
}

// Delete 删除待补充运价
func (s *PendingStore) Delete(ctx context.Context, userID, chatID string) error {
	return s.rdb.Del(ctx, pendingKey(userID, chatID)).Err()
}
