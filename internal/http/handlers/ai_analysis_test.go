package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gavv/httpexpect/v2"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"go-demo/internal/config"
	"go-demo/internal/db"
	"go-demo/internal/sqllog"
)

type AIAnalysisTestSuite struct {
	suite.Suite
	e      *httpexpect.Expect
	server *httptest.Server
	repo   *sqllog.Repository
	dbx    *db.DB
}

func (suite *AIAnalysisTestSuite) SetupSuite() {
	// Setup test configuration
	cfg := config.Config{
		DatabaseURL:  getTestDatabaseURL(),
		MaxBodyBytes: 1024 * 1024,
		OpenAIAPIKey: "", // Empty for tests - will mock responses
	}

	// Setup logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	// Setup database
	var err error
	suite.dbx, err = db.New(cfg, logger)
	require.NoError(suite.T(), err)

	// Setup repository
	suite.repo = sqllog.NewRepository(suite.dbx.Gorm)

	// Create test server with AI analysis handler
	aiAnalysisHandler := NewAIAnalysisHandler(suite.repo, logger, cfg)

	mux := http.NewServeMux()
	mux.Handle("POST /v1/ai-analysis", aiAnalysisHandler.AIAnalysis())

	suite.server = httptest.NewServer(mux)
	suite.e = httpexpect.Default(suite.T(), suite.server.URL)
}

func (suite *AIAnalysisTestSuite) TearDownSuite() {
	suite.server.Close()
	if suite.dbx != nil {
		suite.dbx.Close()
	}
}

func (suite *AIAnalysisTestSuite) SetupTest() {
	// Clean up test data before each test
	err := suite.dbx.Gorm.Exec("DELETE FROM sql_logs").Error
	if err != nil {
		suite.T().Logf("Warning: could not clean sql_logs table: %v", err)
	}
}

// AI Analysis Tests
func (suite *AIAnalysisTestSuite) TestAnalyze_Success() {
	// Insert some test data for analysis
	suite.insertTestDataForAnalysis()

	// Create a valid analysis request
	requestBody := map[string]interface{}{
		"type":     "performance",
		"database": "testdb1",
		"filters": map[string]interface{}{
			"min_exec_time": 100,
		},
	}

	resp := suite.e.POST("/v1/ai-analysis").
		WithJSON(requestBody).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("analysis_id")
	resp.ContainsKey("type")
	resp.ContainsKey("status")
	resp.ContainsKey("results")
	resp.Value("type").String().IsEqual("performance")
	resp.Value("status").String().IsEqual("completed")
}

func (suite *AIAnalysisTestSuite) TestAnalyze_MissingType() {
	requestBody := map[string]interface{}{
		"database": "testdb1",
	}

	suite.e.POST("/v1/ai-analysis").
		WithJSON(requestBody).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		Value("code").String().IsEqual("bad_request")
}

func (suite *AIAnalysisTestSuite) TestAnalyze_InvalidType() {
	requestBody := map[string]interface{}{
		"type":     "invalid_type",
		"database": "testdb1",
	}

	suite.e.POST("/v1/ai-analysis").
		WithJSON(requestBody).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		Value("code").String().IsEqual("bad_request")
}

func (suite *AIAnalysisTestSuite) TestAnalyze_MissingDatabase() {
	requestBody := map[string]interface{}{
		"type": "performance",
	}

	suite.e.POST("/v1/ai-analysis").
		WithJSON(requestBody).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		Value("code").String().IsEqual("bad_request")
}

func (suite *AIAnalysisTestSuite) TestAnalyze_EmptyDatabase() {
	// No data in database
	requestBody := map[string]interface{}{
		"type":     "performance",
		"database": "nonexistent_db",
	}

	resp := suite.e.POST("/v1/ai-analysis").
		WithJSON(requestBody).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("analysis_id")
	resp.ContainsKey("status")
	resp.Value("status").String().IsEqual("completed")
	resp.Value("results").Object().Value("queries_analyzed").Number().IsEqual(0)
}

func (suite *AIAnalysisTestSuite) TestAnalyze_InvalidJSON() {
	invalidJSON := `{"type": "performance", "database":}`

	suite.e.POST("/v1/ai-analysis").
		WithBytes([]byte(invalidJSON)).
		WithHeader("Content-Type", "application/json").
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		Value("code").String().IsEqual("bad_request")
}

func (suite *AIAnalysisTestSuite) TestAnalyze_SecurityAnalysis() {
	// Insert some test data with potential security issues
	suite.insertSecurityTestData()

	requestBody := map[string]interface{}{
		"type":     "security",
		"database": "testdb1",
	}

	resp := suite.e.POST("/v1/ai-analysis").
		WithJSON(requestBody).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("analysis_id")
	resp.Value("type").String().IsEqual("security")
	resp.Value("status").String().IsEqual("completed")
	resp.ContainsKey("results")
}

func (suite *AIAnalysisTestSuite) TestAnalyze_OptimizationAnalysis() {
	// Insert some test data for optimization analysis
	suite.insertOptimizationTestData()

	requestBody := map[string]interface{}{
		"type":     "optimization",
		"database": "testdb1",
		"filters": map[string]interface{}{
			"min_exec_count": 10,
		},
	}

	resp := suite.e.POST("/v1/ai-analysis").
		WithJSON(requestBody).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("analysis_id")
	resp.Value("type").String().IsEqual("optimization")
	resp.Value("status").String().IsEqual("completed")
	resp.ContainsKey("results")
}

func (suite *AIAnalysisTestSuite) TestAnalyze_WithComplexFilters() {
	// Insert varied test data
	suite.insertVariedTestData()

	requestBody := map[string]interface{}{
		"type":     "performance",
		"database": "testdb1",
		"filters": map[string]interface{}{
			"min_exec_time":  200,
			"max_exec_time":  1000,
			"min_exec_count": 5,
			"query_pattern":  "SELECT",
		},
	}

	resp := suite.e.POST("/v1/ai-analysis").
		WithJSON(requestBody).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("analysis_id")
	resp.Value("type").String().IsEqual("performance")
	resp.ContainsKey("results")
}

func (suite *AIAnalysisTestSuite) TestAnalyze_LargePayload() {
	// Create a large payload to test size limits
	largeFilters := make(map[string]interface{})
	for i := 0; i < 1000; i++ {
		largeFilters[fmt.Sprintf("filter_%d", i)] = "value"
	}

	requestBody := map[string]interface{}{
		"type":     "performance",
		"database": "testdb1",
		"filters":  largeFilters,
	}

	// This should either succeed or fail gracefully depending on server limits
	resp := suite.e.POST("/v1/ai-analysis").
		WithJSON(requestBody).
		Expect()

	// Accept either success or request entity too large
	status := resp.Raw().StatusCode
	suite.True(status == http.StatusOK || status == http.StatusRequestEntityTooLarge || status == http.StatusBadRequest)
}

// Helper methods
func (suite *AIAnalysisTestSuite) insertTestDataForAnalysis() {
	logs := []sqllog.SQLLog{
		{
			DBName:     "testdb1",
			SQLQuery:   "SELECT * FROM users WHERE id = ?",
			ExecTimeMs: 150,
			ExecCount:  25,
		},
		{
			DBName:     "testdb1",
			SQLQuery:   "SELECT * FROM orders WHERE user_id = ? AND status = ?",
			ExecTimeMs: 300,
			ExecCount:  45,
		},
		{
			DBName:     "testdb1",
			SQLQuery:   "UPDATE users SET last_login = NOW() WHERE id = ?",
			ExecTimeMs: 50,
			ExecCount:  100,
		},
	}

	err := suite.repo.InsertBatch(context.Background(), logs)
	require.NoError(suite.T(), err)
}

func (suite *AIAnalysisTestSuite) insertSecurityTestData() {
	logs := []sqllog.SQLLog{
		{
			DBName:     "testdb1",
			SQLQuery:   "SELECT * FROM users WHERE username = 'admin' AND password = 'password123'",
			ExecTimeMs: 100,
			ExecCount:  5,
		},
		{
			DBName:     "testdb1",
			SQLQuery:   "SELECT * FROM sensitive_data WHERE user_id = 1",
			ExecTimeMs: 80,
			ExecCount:  15,
		},
	}

	err := suite.repo.InsertBatch(context.Background(), logs)
	require.NoError(suite.T(), err)
}

func (suite *AIAnalysisTestSuite) insertOptimizationTestData() {
	logs := []sqllog.SQLLog{
		{
			DBName:     "testdb1",
			SQLQuery:   "SELECT * FROM large_table ORDER BY created_at DESC LIMIT 10",
			ExecTimeMs: 500,
			ExecCount:  50,
		},
		{
			DBName:     "testdb1",
			SQLQuery:   "SELECT COUNT(*) FROM users WHERE status = 'active'",
			ExecTimeMs: 200,
			ExecCount:  30,
		},
	}

	err := suite.repo.InsertBatch(context.Background(), logs)
	require.NoError(suite.T(), err)
}

func (suite *AIAnalysisTestSuite) insertVariedTestData() {
	logs := []sqllog.SQLLog{
		{
			DBName:     "testdb1",
			SQLQuery:   "SELECT * FROM users",
			ExecTimeMs: 100,
			ExecCount:  10,
		},
		{
			DBName:     "testdb1",
			SQLQuery:   "SELECT * FROM orders",
			ExecTimeMs: 250,
			ExecCount:  8,
		},
		{
			DBName:     "testdb1",
			SQLQuery:   "INSERT INTO logs VALUES (?)",
			ExecTimeMs: 50,
			ExecCount:  20,
		},
		{
			DBName:     "testdb1",
			SQLQuery:   "DELETE FROM temp_data",
			ExecTimeMs: 800,
			ExecCount:  3,
		},
	}

	err := suite.repo.InsertBatch(context.Background(), logs)
	require.NoError(suite.T(), err)
}

func TestAIAnalysisTestSuite(t *testing.T) {
	suite.Run(t, new(AIAnalysisTestSuite))
}
