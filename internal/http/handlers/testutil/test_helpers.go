package testutil

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gavv/httpexpect/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"go-demo/internal/auth"
	"go-demo/internal/config"
	"go-demo/internal/db"
	httpServer "go-demo/internal/http"
	"go-demo/internal/sqllog"
)

// TestConfig holds test configuration
type TestConfig struct {
	Config     config.Config
	DB         *gorm.DB
	SqlDB      *sql.DB
	Logger     *slog.Logger
	AuthSvc    *auth.Service
	SqlLogRepo *sqllog.Repository
	Cleanup    func()
}

// TestUser represents a test user with credentials
type TestUser struct {
	ID       string
	Username string
	Email    string
	Password string
	Role     string
	Token    string
}

// SetupTestServer creates a test server with all dependencies configured
func SetupTestServer(t *testing.T) (*httpexpect.Expect, *TestConfig, func()) {
	// Create test configuration
	cfg := config.Config{
		Port:           "8080",
		LogLevel:       "info",
		RequestTimeout: 30 * time.Second,
		MaxBodyBytes:   1024 * 1024, // 1MB
		AllowedOrigins: []string{"*"},
		Env:            "test",
		DatabaseURL:    getTestDatabaseURL(),
		JWTSecret:      "test-jwt-secret-key-for-testing-only",
		JWTTTL:         15 * time.Minute,
		RefreshTTL:     24 * time.Hour,
		OpenAIAPIKey:   "test-openai-key",
	}

	// Setup logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Setup database
	dbx, err := setupTestDatabase(cfg, logger)
	require.NoError(t, err)

	// Initialize auth service
	authSvc := auth.NewService(dbx, cfg, logger)

	// Initialize SQL log repository
	sqlLogRepo := sqllog.NewRepository(dbx.Gorm)

	// Create HTTP server
	handler := httpServer.NewRouter(cfg, logger, authSvc, sqlLogRepo)
	server := httptest.NewServer(handler)

	// Create httpexpect instance
	e := httpexpect.Default(t, server.URL)

	testConfig := &TestConfig{
		Config:     cfg,
		DB:         dbx.Gorm,
		SqlDB:      dbx.SQL,
		Logger:     logger,
		AuthSvc:    authSvc,
		SqlLogRepo: sqlLogRepo,
		Cleanup: func() {
			server.Close()
			dbx.Close()
		},
	}

	return e, testConfig, testConfig.Cleanup
}

// getTestDatabaseURL returns the test database URL
func getTestDatabaseURL() string {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		// Default test database URL - adjust as needed
		dbURL = "postgres://postgres:postgres@localhost:5432/go_demo_test?sslmode=disable"
	}
	return dbURL
}

// setupTestDatabase creates and configures a test database connection
func setupTestDatabase(cfg config.Config, logger *slog.Logger) (*db.DB, error) {
	// Use the existing db.New function which handles migration
	dbx, err := db.New(cfg, logger)
	if err != nil {
		return nil, err
	}
	return dbx, nil
}

// CreateTestUser creates a test user and returns user details with JWT token
func CreateTestUser(t *testing.T, testConfig *TestConfig, username, email, password, role string) *TestUser {
	ctx := context.Background()

	// Create user via auth service
	user, err := testConfig.AuthSvc.Register(ctx, username, email, password, "test-admin")
	require.NoError(t, err)

	// Update role if not USER
	if role != "USER" {
		err = testConfig.DB.Model(&user).Update("role", role).Error
		require.NoError(t, err)
	}

	// Login to get token
	user2, accessToken, _, _, _, err := testConfig.AuthSvc.Login(ctx, email, password)
	require.NoError(t, err)

	return &TestUser{
		ID:       user2.ID,
		Username: user2.Username,
		Email:    user2.Email,
		Password: password,
		Role:     user2.Role,
		Token:    accessToken,
	}
}

// CleanupTestData removes all test data from the database
func CleanupTestData(t *testing.T, db *gorm.DB) {
	// Clean up in reverse order of dependencies
	tables := []string{
		"DEMO.REFRESH_TOKEN",
		"DEMO.USER",
		"sql_logs", // Might be in default schema
	}

	for _, table := range tables {
		err := db.Exec("DELETE FROM " + table).Error
		if err != nil {
			// Log but don't fail test - table might not exist
			t.Logf("Warning: could not clean table %s: %v", table, err)
		}
	}
}

// AuthHeader returns authorization header with Bearer token
func AuthHeader(token string) string {
	return "Bearer " + token
}

// SeedTestRoles ensures test roles exist in the database
func SeedTestRoles(t *testing.T, database *gorm.DB) {
	roles := []db.Role{
		{Code: "ADMIN", Name: "Administrator", Description: "Full system access"},
		{Code: "USER", Name: "User", Description: "Standard user access"},
		{Code: "ANALYZER", Name: "Analyzer", Description: "Data analysis access"},
		{Code: "MONITOR", Name: "Monitor", Description: "Monitoring access"},
		{Code: "TEAM_LEADER", Name: "Team Leader", Description: "Team management access"},
	}

	for _, role := range roles {
		// Use ON CONFLICT DO NOTHING equivalent for GORM
		var existing db.Role
		err := database.Where("code = ?", role.Code).First(&existing).Error
		if err != nil {
			// Role doesn't exist, create it
			role.CreatedBy = "test-system"
			role.UpdatedBy = "test-system"
			err = database.Create(&role).Error
			require.NoError(t, err)
		}
	}
}
