package sqllog

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Migrate ensures the DEMO.SQL_LOG table exists.
func (r *Repository) Migrate(ctx context.Context) error {
	return r.db.WithContext(ctx).AutoMigrate(&SQLLog{})
}

// InsertBatch inserts entries in batches for performance.
func (r *Repository) InsertBatch(ctx context.Context, entries []SQLLog) error {
	if len(entries) == 0 {
		return nil
	}
	// Basic validation safeguard before hitting DB
	for i := range entries {
		if entries[i].DBName == "" || entries[i].SQLQuery == "" {
			return fmt.Errorf("missing required fields at index %d", i)
		}
	}
	return r.db.WithContext(ctx).CreateInBatches(entries, 500).Error
}

// Abnormal thresholds as per requirement
const (
	AbnormalExecTimeThreshold  int64 = 500
	AbnormalExecCountThreshold int64 = 100
)

// CountAbnormal returns the total number of abnormal queries
// defined by exec_time_ms > 500 AND exec_count > 100.
func (r *Repository) CountAbnormal(ctx context.Context) (int64, error) {
	var cnt int64
	err := r.db.WithContext(ctx).
		Model(&SQLLog{}).
		Where("exec_time_ms > ? AND exec_count > ?", AbnormalExecTimeThreshold, AbnormalExecCountThreshold).
		Count(&cnt).Error
	return cnt, err
}

// ListAbnormal returns up to 'limit' abnormal queries ordered by
// exec_time_ms DESC, exec_count DESC. Limit is clamped to [1,1000] with default 100.
func (r *Repository) ListAbnormal(ctx context.Context, limit int) ([]SQLLog, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	var items []SQLLog
	err := r.db.WithContext(ctx).
		Where("exec_time_ms > ? AND exec_count > ?", AbnormalExecTimeThreshold, AbnormalExecCountThreshold).
		Order("exec_time_ms DESC, exec_count DESC").
		Limit(limit).
		Find(&items).Error
	return items, err
}
