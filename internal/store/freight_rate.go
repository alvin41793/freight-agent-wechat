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
	ID                uint64     `gorm:"primaryKey;autoIncrement"`
	Agent             string     `gorm:"column:agent;size:128"` // 代理名称(productSource)
	Carrier           string     `gorm:"column:carrier;size:64"`
	POL               string     `gorm:"column:pol;size:128;not null"`
	POLCode           string     `gorm:"column:pol_code;size:10"`
	POD               string     `gorm:"column:pod;size:128;not null"`
	PODCode           string     `gorm:"column:pod_code;size:10"`
	F20GP             string     `gorm:"column:f20gp;size:32"`
	F40GP             string     `gorm:"column:f40gp;size:32"`
	F40HC             string     `gorm:"column:f40hc;size:32"`
	ValidityStartTime string     `gorm:"column:validity_start_time;size:16"`
	ValidityEndTime   string     `gorm:"column:validity_end_time;size:16"`
	ETD               string     `gorm:"column:etd;size:16"`
	ETA               string     `gorm:"column:eta;size:16"`
	POLPODTT          string     `gorm:"column:pol_pod_tt;size:16"`
	Vessel            string     `gorm:"column:vessel;size:128"`
	Voyage            string     `gorm:"column:voyage;size:64"`
	CutOffTime        string     `gorm:"column:cut_off_time;size:16"`
	PortClosingTime   string     `gorm:"column:port_closing_time;size:16"`
	Commodity         string     `gorm:"column:commodity;size:255"`
	DND               string     `gorm:"column:dnd;size:128"`
	WeightLimit       string     `gorm:"column:weight_limit;size:64"`
	Remark            string     `gorm:"column:remark;type:text"`
	PushStatus        int8       `gorm:"column:push_status;default:0"` // 推送状态: 0-未推送 1-推送成功 2-推送失败
	PushError         string     `gorm:"column:push_error;type:text"`  // 推送错误信息
	PushedAt          *time.Time `gorm:"column:pushed_at"`             // 推送成功时间
	SourceUserID      string     `gorm:"column:source_user_id;size:64"`
	SourceChatID      string     `gorm:"column:source_chat_id;size:64"`
	CreatedAt         time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;autoUpdateTime"`
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

// BatchSave 批量保存运价记录，返回保存后的记录 ID 列表
func (s *FreightRateStore) BatchSave(ctx context.Context, rates []llm.FreightRate, userID, chatID string) ([]uint64, error) {
	if len(rates) == 0 {
		return nil, nil
	}

	records := make([]FreightRateRecord, 0, len(rates))
	for _, r := range rates {
		records = append(records, FreightRateRecord{
			Agent:             r.Agent,
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
		return nil, fmt.Errorf("batch save freight rates: %w", result.Error)
	}

	// 提取保存后的 ID 列表
	ids := make([]uint64, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return ids, nil
}

// UpdatePushStatus 更新运价记录的推送状态
// 注意: 这个方法需要根据实际的 ID 映射来更新,这里简化处理
// 实际应用中应该通过 rateStore 返回保存后的 ID 列表
func (s *FreightRateStore) UpdatePushStatus(ctx context.Context, successIDs []uint64, failedIDs []uint64, errorMsg string) error {
	if len(successIDs) > 0 {
		now := time.Now()
		if err := s.db.WithContext(ctx).Model(&FreightRateRecord{}).
			Where("id IN ?", successIDs).
			Updates(map[string]interface{}{
				"push_status": 1,
				"pushed_at":   now,
				"push_error":  "",
			}).Error; err != nil {
			return fmt.Errorf("update success status: %w", err)
		}
	}

	if len(failedIDs) > 0 {
		if err := s.db.WithContext(ctx).Model(&FreightRateRecord{}).
			Where("id IN ?", failedIDs).
			Updates(map[string]interface{}{
				"push_status": 2,
				"push_error":  errorMsg,
			}).Error; err != nil {
			return fmt.Errorf("update failed status: %w", err)
		}
	}

	return nil
}
