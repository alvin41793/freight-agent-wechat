package store

import (
	"context"
	"fmt"
	"time"

	"freight-agent-wechat/internal/llm"

	"gorm.io/gorm"
)

// FreightRateRecord GORM 模型，对应 freight_rates 表
// size 标签必须与迁移 SQL 中的列定义保持一致，防止 GORM AutoMigrate 将 VARCHAR 改为 longtext 出现索引错误
type FreightRateRecord struct {
	ID                uint64    `gorm:"primaryKey;autoIncrement"`
	Carrier           string    `gorm:"column:carrier;size:64"`
	POL               string    `gorm:"column:pol;size:128;not null"`
	POLCode           string    `gorm:"column:pol_code;size:10"`
	POD               string    `gorm:"column:pod;size:128;not null"`
	PODCode           string    `gorm:"column:pod_code;size:10"`
	F20GP             string    `gorm:"column:f20gp;size:32"`
	F40GP             string    `gorm:"column:f40gp;size:32"`
	F40HC             string    `gorm:"column:f40hc;size:32"`
	ValidityStartTime string    `gorm:"column:validity_start_time;size:16"`
	ValidityEndTime   string    `gorm:"column:validity_end_time;size:16"`
	ETD               string    `gorm:"column:etd;size:16"`
	ETA               string    `gorm:"column:eta;size:16"`
	POLPODTT          string    `gorm:"column:pol_pod_tt;size:16"`
	Vessel            string    `gorm:"column:vessel;size:128"`
	Voyage            string    `gorm:"column:voyage;size:64"`
	CutOffTime        string    `gorm:"column:cut_off_time;size:16"`
	PortClosingTime   string    `gorm:"column:port_closing_time;size:16"`
	Commodity         string    `gorm:"column:commodity;size:255"`
	DND               string    `gorm:"column:dnd;size:128"`
	WeightLimit       string    `gorm:"column:weight_limit;size:64"`
	Remark            string    `gorm:"column:remark;type:text"`
	SourceUserID      string    `gorm:"column:source_user_id;size:64"`
	SourceChatID      string    `gorm:"column:source_chat_id;size:64"`
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 指定表名
func (FreightRateRecord) TableName() string { return "freight_rates" }

// FreightRateStore MySQL 运价存储
type FreightRateStore struct {
	db *gorm.DB
}

// NewFreightRateStore 创建运价存储
func NewFreightRateStore(db *gorm.DB) *FreightRateStore {
	return &FreightRateStore{db: db}
}

// BatchSave 批量保存运价记录
func (s *FreightRateStore) BatchSave(ctx context.Context, rates []llm.FreightRate, userID, chatID string) error {
	if len(rates) == 0 {
		return nil
	}

	records := make([]FreightRateRecord, 0, len(rates))
	for _, r := range rates {
		records = append(records, FreightRateRecord{
			Carrier:           r.Carrier,
			POL:               r.POL,
			POLCode:           r.POLCode,
			POD:               r.POD,
			PODCode:           r.PODCode,
			F20GP:             r.F20GP,
			F40GP:             r.F40GP,
			F40HC:             r.F40HC,
			ValidityStartTime: r.ValidityStartTime,
			ValidityEndTime:   r.ValidityEndTime,
			ETD:               r.ETD,
			ETA:               r.ETA,
			POLPODTT:          r.POLPODTT,
			Vessel:            r.Vessel,
			Voyage:            r.Voyage,
			CutOffTime:        r.CutOffTime,
			PortClosingTime:   r.PortClosingTime,
			Commodity:         r.Commodity,
			DND:               r.DND,
			WeightLimit:       r.WeightLimit,
			Remark:            r.Remark,
			SourceUserID:      userID,
			SourceChatID:      chatID,
		})
	}

	if result := s.db.WithContext(ctx).Create(&records); result.Error != nil {
		return fmt.Errorf("batch save freight rates: %w", result.Error)
	}
	return nil
}
