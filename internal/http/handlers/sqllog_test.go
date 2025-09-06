package handlers

import (
	"context"
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

type SQLLogTestSuite struct {
	suite.Suite
	e      *httpexpect.Expect
	server *httptest.Server
	repo   *sqllog.Repository
	dbx    *db.DB
}

func (suite *SQLLogTestSuite) SetupSuite() {
	// Setup test configuration
	cfg := config.Config{
		DatabaseURL:  getTestDatabaseURL(),
		MaxBodyBytes: 1024 * 1024,
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

	// Create test server with SQL log handlers
	uploadHandler := NewSQLLogUpload(suite.repo, logger, cfg.MaxBodyBytes)
	queryHandler := NewSQLLogQuery(suite.repo, logger)
	scanHandler := NewSQLLogScan(suite.repo, logger)

	mux := http.NewServeMux()
	mux.Handle("POST /v1/sql-logs/upload", uploadHandler.Upload())
	mux.Handle("GET /v1/sql-logs/databases", queryHandler.ListDatabases())
	mux.Handle("GET /v1/sql-logs", queryHandler.ListByDB())
	mux.Handle("GET /v1/sql-logs/scan", scanHandler.Scan())

	suite.server = httptest.NewServer(mux)
	suite.e = httpexpect.Default(suite.T(), suite.server.URL)
}

func (suite *SQLLogTestSuite) TearDownSuite() {
	suite.server.Close()
	if suite.dbx != nil {
		suite.dbx.Close()
	}
}

func (suite *SQLLogTestSuite) SetupTest() {
	// Clean up test data before each test
	err := suite.dbx.Gorm.Exec("DELETE FROM sql_logs").Error
	if err != nil {
		suite.T().Logf("Warning: could not clean sql_logs table: %v", err)
	}
}

// Upload Tests
func (suite *SQLLogTestSuite) TestUpload_Success() {
	// Create a test log file content
	logContent := `2024-09-06 12:00:00,123 [db1] SELECT * FROM users WHERE id = 1 | 150ms | 5
2024-09-06 12:01:00,456 [db2] SELECT * FROM orders WHERE user_id = 1 | 200ms | 3`

	resp := suite.e.POST("/v1/sql-logs/upload").
		WithMultipart().
		WithFileBytes("file", "test.log", []byte(logContent)).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.Value("inserted").Number().Gt(0)
	resp.Value("total_lines").Number().Gt(0)
}

func (suite *SQLLogTestSuite) TestUpload_MissingFile() {
	suite.e.POST("/v1/sql-logs/upload").
		WithMultipart().
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		Value("code").String().IsEqual("bad_request")
}

func (suite *SQLLogTestSuite) TestUpload_InvalidFileType() {
	suite.e.POST("/v1/sql-logs/upload").
		WithMultipart().
		WithFileBytes("file", "test.txt", []byte("some text")).
		Expect().
		Status(http.StatusBadRequest)
}

func (suite *SQLLogTestSuite) TestUpload_EmptyFile() {
	resp := suite.e.POST("/v1/sql-logs/upload").
		WithMultipart().
		WithFileBytes("file", "test.log", []byte("")).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.Value("inserted").Number().IsEqual(0)
	resp.Value("total_lines").Number().IsEqual(0)
}

// ListDatabases Tests
func (suite *SQLLogTestSuite) TestListDatabases_Success() {
	// Insert some test data
	suite.insertTestData()

	resp := suite.e.GET("/v1/sql-logs/databases").
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("databases")
	resp.Value("databases").Array().Length().Gt(0)
}

func (suite *SQLLogTestSuite) TestListDatabases_EmptyResult() {
	// No test data inserted
	resp := suite.e.GET("/v1/sql-logs/databases").
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("databases")
	resp.Value("databases").Array().Length().IsEqual(0)
}

// ListByDB Tests
func (suite *SQLLogTestSuite) TestListByDB_Success() {
	// Insert some test data
	suite.insertTestData()

	resp := suite.e.GET("/v1/sql-logs").
		WithQuery("db", "testdb1").
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("items")
	resp.Value("items").Array().Length().Gt(0)
}

func (suite *SQLLogTestSuite) TestListByDB_MissingDBParam() {
	suite.e.GET("/v1/sql-logs").
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		Value("code").String().IsEqual("bad_request")
}

func (suite *SQLLogTestSuite) TestListByDB_NoResults() {
	resp := suite.e.GET("/v1/sql-logs").
		WithQuery("db", "nonexistent").
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("message")
	resp.ContainsKey("items")
	resp.Value("items").Array().Length().IsEqual(0)
}

// Scan Tests
func (suite *SQLLogTestSuite) TestScan_Success() {
	// Insert some abnormal test data (exec_time_ms > 500 AND exec_count > 100)
	suite.insertAbnormalTestData()

	resp := suite.e.GET("/v1/sql-logs/scan").
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("total")
	resp.ContainsKey("items")
	resp.Value("total").Number().Gt(0)
	resp.Value("items").Array().Length().Gt(0)
}

func (suite *SQLLogTestSuite) TestScan_NoAbnormalQueries() {
	// Insert only normal test data
	suite.insertTestData()

	resp := suite.e.GET("/v1/sql-logs/scan").
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("message")
	resp.Value("total").Number().IsEqual(0)
	resp.Value("items").Array().Length().IsEqual(0)
}

func (suite *SQLLogTestSuite) TestScan_WithLimit() {
	// Insert some abnormal test data
	suite.insertAbnormalTestData()

	resp := suite.e.GET("/v1/sql-logs/scan").
		WithQuery("limit", "5").
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("items")
	resp.Value("items").Array().Length().Le(5)
}

func (suite *SQLLogTestSuite) TestScan_InvalidLimit() {
	suite.e.GET("/v1/sql-logs/scan").
		WithQuery("limit", "invalid").
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		Value("code").String().IsEqual("bad_request")
}

// Helper methods
func (suite *SQLLogTestSuite) insertTestData() {
	logs := []sqllog.SQLLog{
		{
			DBName:     "testdb1",
			SQLQuery:   "SELECT * FROM users WHERE id = 1",
			ExecTimeMs: 150,
			ExecCount:  5,
		},
		{
			DBName:     "testdb2",
			SQLQuery:   "SELECT * FROM orders WHERE user_id = 1",
			ExecTimeMs: 200,
			ExecCount:  3,
		},
	}

	err := suite.repo.InsertBatch(context.Background(), logs)
	require.NoError(suite.T(), err)
}

func (suite *SQLLogTestSuite) insertAbnormalTestData() {
	logs := []sqllog.SQLLog{
		{
			DBName:     "testdb1",
			SQLQuery:   "SELECT * FROM large_table WHERE complex_condition = 1",
			ExecTimeMs: 1500, // > 500
			ExecCount:  150,  // > 100
		},
		{
			DBName:     "testdb2",
			SQLQuery:   "SELECT * FROM another_large_table JOIN other_table",
			ExecTimeMs: 2000, // > 500
			ExecCount:  200,  // > 100
		},
	}

	err := suite.repo.InsertBatch(context.Background(), logs)
	require.NoError(suite.T(), err)
}

func TestSQLLogTestSuite(t *testing.T) {
	suite.Run(t, new(SQLLogTestSuite))
}
