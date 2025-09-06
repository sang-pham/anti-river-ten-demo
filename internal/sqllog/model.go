package sqllog

import "time"

// SQLLog represents one parsed log record from logsql.log.
type SQLLog struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	DBName     string    `gorm:"column:db_name;type:text;not null"`
	SQLQuery   string    `gorm:"column:sql_query;type:text;not null"`
	ExecTimeMs int64     `gorm:"column:exec_time_ms;not null"`
	ExecCount  int64     `gorm:"column:exec_count;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName returns the fully qualified table under DEMO schema.
func (SQLLog) TableName() string {
	return "DEMO.SQL_LOG"
}
