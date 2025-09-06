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
