package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// Store 会话存储接口
type Store interface {
	Get(ctx context.Context, sessionKey string) (*QuoteSession, error)
	Set(ctx context.Context, session *QuoteSession, ttl time.Duration) error
	Delete(ctx context.Context, sessionKey string) error
	Exists(ctx context.Context, sessionKey string) (bool, error)
}

// RedisStore Redis 存储实现
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore 创建 Redis 存储
func NewRedisStore(addr, password string, db int) *RedisStore {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	return &RedisStore{client: client}
}

// Get 获取会话
func (s *RedisStore) Get(ctx context.Context, sessionKey string) (*QuoteSession, error) {
	data, err := s.client.Get(ctx, sessionKey).Result()
	if err == redis.Nil {
		return nil, nil // 会话不存在
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session from redis: %w", err)
	}

	var session QuoteSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// Set 保存会话
func (s *RedisStore) Set(ctx context.Context, session *QuoteSession, ttl time.Duration) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := s.client.Set(ctx, session.SessionID, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to set session in redis: %w", err)
	}

	return nil
}

// Delete 删除会话
func (s *RedisStore) Delete(ctx context.Context, sessionKey string) error {
	if err := s.client.Del(ctx, sessionKey).Err(); err != nil {
		return fmt.Errorf("failed to delete session from redis: %w", err)
	}
	return nil
}

// Exists 检查会话是否存在
func (s *RedisStore) Exists(ctx context.Context, sessionKey string) (bool, error) {
	n, err := s.client.Exists(ctx, sessionKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check session existence: %w", err)
	}
	return n > 0, nil
}

// Ping 检查 Redis 连接
func (s *RedisStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

// Close 关闭连接
func (s *RedisStore) Close() error {
	return s.client.Close()
}
