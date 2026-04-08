package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
)

// ── 步骤名常量 ──────────────────────────────────────────────────────────────

const (
	StepIntentCheck = "intent_check" // 意图识别（规则判断）
	StepLLMParse    = "llm_parse"    // LLM 运价提取
	StepValidate    = "validate"     // 字段校验（有效/无效计数）
	StepDBSave      = "db_save"      // 写入数据库
)

// ── 状态常量 ────────────────────────────────────────────────────────────────

const (
	TaskStatusPending   = "pending"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
	TaskStatusRejected  = "rejected" // 意图识别拒绝

	StepStatusSuccess = "success"
	StepStatusFailed  = "failed"
	StepStatusSkipped = "skipped"
)

// ── GORM 模型 ────────────────────────────────────────────────────────────────

// TaskRecord 对应 tasks 表
type TaskRecord struct {
	ID              string     `gorm:"primaryKey;type:varchar(36)"`
	UserID          string     `gorm:"column:user_id;size:64;not null"`
	ChatID          string     `gorm:"column:chat_id;size:64"`
	RawText         string     `gorm:"column:raw_text;type:text;not null"`
	Status          string     `gorm:"column:status;size:16;not null;default:pending"`
	TotalDurationMS *int64     `gorm:"column:total_duration_ms"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime"`
	CompletedAt     *time.Time `gorm:"column:completed_at"`
}

func (TaskRecord) TableName() string { return "tasks" }

// TaskStepRecord 对应 task_steps 表
type TaskStepRecord struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	TaskID     string    `gorm:"column:task_id;not null;type:varchar(36)"`
	Step       string    `gorm:"column:step;size:64;not null"`
	Status     string    `gorm:"column:status;size:16;not null"`
	Input      string    `gorm:"column:input;type:text"`
	Output     string    `gorm:"column:output;type:text"`
	DurationMS *int64    `gorm:"column:duration_ms"`
	Error      string    `gorm:"column:error;type:text"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (TaskStepRecord) TableName() string { return "task_steps" }

// ── TaskStore ────────────────────────────────────────────────────────────────

// TaskStore 负责任务与步骤的持久化
type TaskStore struct {
	db *gorm.DB
}

// NewTaskStore 创建 TaskStore
func NewTaskStore(db *gorm.DB) *TaskStore {
	return &TaskStore{db: db}
}

// Create 创建任务（状态为 pending）
func (s *TaskStore) Create(ctx context.Context, task *TaskRecord) error {
	if err := s.db.WithContext(ctx).Create(task).Error; err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	return nil
}

// AddStep 写入一条步骤记录，失败时仅记录日志不返回 error（不阻塞主流程）
func (s *TaskStore) AddStep(ctx context.Context, step *TaskStepRecord) {
	if err := s.db.WithContext(ctx).Create(step).Error; err != nil {
		log.Printf("[task] add step %q error: %v", step.Step, err)
	}
}

// Complete 更新任务为最终状态
func (s *TaskStore) Complete(ctx context.Context, taskID, status string, totalMS int64) {
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&TaskRecord{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":            status,
			"total_duration_ms": totalMS,
			"completed_at":      now,
		}).Error; err != nil {
		log.Printf("[task] complete task %s error: %v", taskID, err)
	}
}

// ── 辅助函数 ─────────────────────────────────────────────────────────────────

// NewTaskID 生成 UUID v4 格式的任务ID
func NewTaskID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	// 设置 version 4 和 variant bits
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}

// Int64Ptr 将 int64 转为指针（用于可空字段赋值）
func Int64Ptr(v int64) *int64 { return &v }

// TruncateText 截取文本前 n 个字符（按 rune），用于存储输入摘要
func TruncateText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
