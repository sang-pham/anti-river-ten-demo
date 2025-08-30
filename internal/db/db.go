package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"go-demo/internal/config"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// DB wraps gorm.DB with an underlying *sql.DB for pooling controls and Close.
type DB struct {
	Gorm *gorm.DB
	SQL  *sql.DB
	log  *slog.Logger
}

// New opens a PostgreSQL connection using GORM and runs AutoMigrate.
func New(cfg config.Config, log *slog.Logger) (*DB, error) {
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	g, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  cfg.DatabaseURL,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			// Set default schema to DEMO for all tables.
			TablePrefix:   "DEMO.",
			SingularTable: false,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	sqlDB, err := g.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}

	// Sensible pool defaults; could be moved to config later.
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(25)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	sqlDB.SetConnMaxLifetime(60 * time.Minute)

	// Ensure DEMO schema exists
	if err := g.Exec(`CREATE SCHEMA IF NOT EXISTS "DEMO"`).Error; err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	// AutoMigrate role, user, and refresh token tables in DEMO schema (respect FK order)
	if err := g.AutoMigrate(&Role{}, &User{}, &RefreshToken{}); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	return &DB{Gorm: g, SQL: sqlDB, log: log}, nil
}

// Close closes the underlying sql.DB.
func (d *DB) Close() error {
	if d == nil || d.SQL == nil {
		return nil
	}
	return d.SQL.Close()
}

// Role represents user role mapped to DEMO.ROLE, referenced by User.role.
type Role struct {
	Code        string    `gorm:"column:code;type:varchar(64);primaryKey"`
	Name        string    `gorm:"column:name;type:varchar(128);not null"`
	Description string    `gorm:"column:description;type:text"`
	CreatedBy   string    `gorm:"column:created_by;type:varchar(64)"`
	UpdatedBy   string    `gorm:"column:updated_by;type:varchar(64)"`
	CreatedTime time.Time `gorm:"column:created_time;autoCreateTime"`
	UpdatedTime time.Time `gorm:"column:updated_time;autoUpdateTime"`
}

func (Role) TableName() string { return "DEMO.ROLE" }

// User represents the application user mapped to table "USER".
type User struct {
	ID           string    `gorm:"column:id;type:uuid;primaryKey"`
	Username     string    `gorm:"column:username;type:varchar(64);uniqueIndex;not null"`
	Email        string    `gorm:"column:email;type:varchar(255);uniqueIndex;not null"`
	PasswordHash string    `gorm:"column:password;type:text;not null"`
	CreatedBy    string    `gorm:"column:created_by;type:varchar(64)"`
	UpdatedBy    string    `gorm:"column:updated_by;type:varchar(64)"`
	Role         string    `gorm:"column:role;type:varchar(64);index"` // references Role.code
	CreatedTime  time.Time `gorm:"column:created_time;autoCreateTime"`
	UpdatedTime  time.Time `gorm:"column:updated_time;autoUpdateTime"`

	// Association to enforce FK via AutoMigrate.
	RoleRecord   Role `gorm:"foreignKey:Role;references:Code;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (User) TableName() string { return "DEMO.USER" }

// RefreshToken persists opaque refresh tokens (hashed) for users.
type RefreshToken struct {
	ID          string    `gorm:"column:id;type:uuid;primaryKey"`
	UserID      string    `gorm:"column:user_id;type:uuid;index;not null"`
	TokenHash   string    `gorm:"column:token_hash;type:char(64);uniqueIndex;not null"` // sha256 hex
	ExpiresAt   time.Time `gorm:"column:expires_at;not null"`
	CreatedTime time.Time `gorm:"column:created_time;autoCreateTime"`

	// FK to User
	User User `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (RefreshToken) TableName() string { return "DEMO.REFRESH_TOKEN" }

// BeforeCreate hook to ensure UUID primary key is set.
func (rt *RefreshToken) BeforeCreate(tx *gorm.DB) error {
	if rt.ID == "" {
		rt.ID = uuid.NewString()
	}
	return nil
}

// BeforeCreate hook to ensure UUID primary key is set.
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	return nil
}