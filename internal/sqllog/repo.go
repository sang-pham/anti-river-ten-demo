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

// ListDatabases returns distinct database names present in the log table.
func (r *Repository) ListDatabases(ctx context.Context) ([]string, error) {
	var names []string
	err := r.db.WithContext(ctx).Model(&SQLLog{}).Distinct().Pluck("db_name", &names).Error
	return names, err
}

// FindByDB returns all SQL log entries for a specific database.
func (r *Repository) FindByDB(ctx context.Context, dbName string) ([]SQLLog, error) {
	var rows []SQLLog
	err := r.db.WithContext(ctx).
		Where("db_name = ?", dbName).
		Order("created_at DESC, id DESC").
		Find(&rows).Error
	return rows, err
}

// FindSlowQueries returns SQL queries that are slow and frequently executed
func (r *Repository) FindSlowQueries(ctx context.Context, dbName string) ([]SQLLog, error) {
	var results []SQLLog
	err := r.db.WithContext(ctx).
		Where("db_name = ? AND exec_time_ms > ? AND exec_count > ?", dbName, 500, 100).
		Find(&results).Error
	return results, err
}

// ListDatabases returns distinct database names present in the log table.
func (r *Repository) ListDatabases(ctx context.Context) ([]string, error) {
	var names []string
	err := r.db.WithContext(ctx).Model(&SQLLog{}).Distinct().Pluck("db_name", &names).Error
	return names, err
}

// FindByDB returns all SQL log entries for a specific database.
func (r *Repository) FindByDB(ctx context.Context, dbName string) ([]SQLLog, error) {
	var rows []SQLLog
	err := r.db.WithContext(ctx).
		Where("db_name = ?", dbName).
		Order("created_at DESC, id DESC").
		Find(&rows).Error
	return rows, err
}
