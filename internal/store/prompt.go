package store

import (
	"time"

	"freight-agent-wechat/internal/llm"

	"gorm.io/gorm"
)

// PromptTemplate 提示词模板模型
type PromptTemplate struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	Name        string    `gorm:"column:name;size:64;not null;uniqueIndex"`
	Version     string    `gorm:"column:version;size:16;not null;default:v1"`
	Content     string    `gorm:"column:content;type:text;not null"`
	Description string    `gorm:"column:description;size:255"`
	IsActive    bool      `gorm:"column:is_active;not null;default:true"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 指定表名
func (PromptTemplate) TableName() string {
	return "prompt_templates"
}

// PromptStore 提示词模板存储
type PromptStore struct {
	db *gorm.DB
}

// NewPromptStore 创建提示词存储实例
func NewPromptStore(db *gorm.DB) *PromptStore {
	return &PromptStore{db: db}
}

// GetActivePrompt 获取激活状态的提示词
func (s *PromptStore) GetActivePrompt(name string) (string, error) {
	var template PromptTemplate
	err := s.db.Where("name = ? AND is_active = ?", name, true).
		Order("version DESC").
		First(&template).Error

	if err != nil {
		return "", err
	}

	return template.Content, nil
}

// UpsertPrompt 插入或更新提示词
func (s *PromptStore) UpsertPrompt(name, version, content, description string) error {
	var existing PromptTemplate
	err := s.db.Where("name = ? AND version = ?", name, version).First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		// 不存在则插入
		template := PromptTemplate{
			Name:        name,
			Version:     version,
			Content:     content,
			Description: description,
			IsActive:    true,
		}
		return s.db.Create(&template).Error
	}

	if err != nil {
		return err
	}

	// 存在则更新
	return s.db.Model(&existing).Updates(map[string]interface{}{
		"content":     content,
		"description": description,
		"updated_at":  time.Now(),
	}).Error
}

// SetPromptActive 设置指定版本的提示词为激活状态，并取消其他版本的激活
func (s *PromptStore) SetPromptActive(name, version string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 取消同一名称下所有版本的激活状态
		if err := tx.Model(&PromptTemplate{}).
			Where("name = ?", name).
			Update("is_active", false).Error; err != nil {
			return err
		}

		// 激活指定版本
		return tx.Model(&PromptTemplate{}).
			Where("name = ? AND version = ?", name, version).
			Update("is_active", true).Error
	})
}

// SeedDefaultPrompts 初始化默认提示词（仅在表为空时执行）
func (s *PromptStore) SeedDefaultPrompts() error {
	var count int64
	if err := s.db.Model(&PromptTemplate{}).Count(&count).Error; err != nil {
		return err
	}

	// 已有数据则跳过
	if count > 0 {
		return nil
	}

	defaultPrompts := []PromptTemplate{
		{
			Name:        "extract_system_prompt",
			Version:     "v1",
			Content:     llm.ExtractSystemPromptDefault,
			Description: "海运运价结构化解析专家提示词",
			IsActive:    true,
		},
		{
			Name:        "supplement_system_prompt",
			Version:     "v1",
			Content:     llm.SupplementSystemPromptDefault,
			Description: "海运运价数据补全助手提示词",
			IsActive:    true,
		},
	}

	return s.db.Create(&defaultPrompts).Error
}
