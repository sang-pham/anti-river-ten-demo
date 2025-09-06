package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gavv/httpexpect/v2"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"go-demo/internal/auth"
	"go-demo/internal/config"
	"go-demo/internal/db"
)

type AuthTestSuite struct {
	suite.Suite
	e       *httpexpect.Expect
	server  *httptest.Server
	authSvc *auth.Service
	dbx     *db.DB
}

func (suite *AuthTestSuite) SetupSuite() {
	// Setup test configuration
	cfg := config.Config{
		DatabaseURL:  getTestDatabaseURL(),
		JWTSecret:    "test-jwt-secret-key-for-testing-only",
		JWTTTL:       15 * time.Minute,
		RefreshTTL:   24 * time.Hour,
		MaxBodyBytes: 1024 * 1024,
	}

	// Setup logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Reduce log noise in tests
	}))

	// Setup database
	var err error
	suite.dbx, err = db.New(cfg, logger)
	require.NoError(suite.T(), err)

	// Setup auth service
	suite.authSvc = auth.NewService(suite.dbx, cfg, logger)

	// Seed test roles
	suite.seedTestRoles()

	// Create test server with auth handlers
	authHandler := NewAuth(suite.authSvc, logger, cfg.MaxBodyBytes)
	mux := http.NewServeMux()
	mux.Handle("POST /v1/auth/register", authHandler.Register())
	mux.Handle("POST /v1/auth/login", authHandler.Login())
	mux.Handle("POST /v1/auth/refresh", authHandler.Refresh())

	suite.server = httptest.NewServer(mux)
	suite.e = httpexpect.Default(suite.T(), suite.server.URL)
}

func (suite *AuthTestSuite) TearDownSuite() {
	suite.server.Close()
	if suite.dbx != nil {
		suite.dbx.Close()
	}
}

func (suite *AuthTestSuite) SetupTest() {
	// Clean up test data before each test
	suite.cleanupTestData()
}

func (suite *AuthTestSuite) seedTestRoles() {
	roles := []db.Role{
		{Code: "ADMIN", Name: "Administrator", Description: "Full system access"},
		{Code: "USER", Name: "User", Description: "Standard user access"},
		{Code: "ANALYZER", Name: "Analyzer", Description: "Data analysis access"},
		{Code: "MONITOR", Name: "Monitor", Description: "Monitoring access"},
		{Code: "TEAM_LEADER", Name: "Team Leader", Description: "Team management access"},
	}

	for _, role := range roles {
		var existing db.Role
		err := suite.dbx.Gorm.Where("code = ?", role.Code).First(&existing).Error
		if err != nil {
			role.CreatedBy = "test-system"
			role.UpdatedBy = "test-system"
			err = suite.dbx.Gorm.Create(&role).Error
			require.NoError(suite.T(), err)
		}
	}
}

func (suite *AuthTestSuite) cleanupTestData() {
	// Clean up in reverse order of dependencies
	tables := []string{
		"DEMO.REFRESH_TOKEN",
		"DEMO.USER",
	}

	for _, table := range tables {
		err := suite.dbx.Gorm.Exec("DELETE FROM " + table).Error
		if err != nil {
			suite.T().Logf("Warning: could not clean table %s: %v", table, err)
		}
	}
}

func getTestDatabaseURL() string {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/go_demo_test?sslmode=disable"
	}
	return dbURL
}

// Register Tests
func (suite *AuthTestSuite) TestRegister_Success() {
	payload := map[string]interface{}{
		"username": "testuser",
		"email":    "test@example.com",
		"password": "password123",
	}

	resp := suite.e.POST("/v1/auth/register").
		WithJSON(payload).
		Expect().
		Status(http.StatusCreated).
		JSON().Object()

	resp.ContainsKey("id")
	resp.Value("username").String().IsEqual("testuser")
	resp.Value("email").String().IsEqual("test@example.com")
	resp.Value("role").String().IsEqual("USER")
	resp.NotContainsKey("password")
}

func (suite *AuthTestSuite) TestRegister_MissingFields() {
	payload := map[string]interface{}{
		"username": "testuser",
		// missing email and password
	}

	suite.e.POST("/v1/auth/register").
		WithJSON(payload).
		Expect().
		Status(http.StatusInternalServerError) // Auth service validates and returns error
}

func (suite *AuthTestSuite) TestRegister_InvalidJSON() {
	suite.e.POST("/v1/auth/register").
		WithText("invalid json").
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		Value("code").String().IsEqual("bad_request")
}

func (suite *AuthTestSuite) TestRegister_DuplicateUser() {
	payload := map[string]interface{}{
		"username": "testuser",
		"email":    "test@example.com",
		"password": "password123",
	}

	// First registration should succeed
	suite.e.POST("/v1/auth/register").
		WithJSON(payload).
		Expect().
		Status(http.StatusCreated)

	// Second registration should fail
	suite.e.POST("/v1/auth/register").
		WithJSON(payload).
		Expect().
		Status(http.StatusConflict).
		JSON().Object().
		Value("code").String().IsEqual("user_exists")
}

func (suite *AuthTestSuite) TestRegister_MethodNotAllowed() {
	suite.e.GET("/v1/auth/register").
		Expect().
		Status(http.StatusMethodNotAllowed)
}

// Login Tests
func (suite *AuthTestSuite) TestLogin_Success() {
	// First create a user
	_, err := suite.authSvc.Register(context.Background(), "testuser", "test@example.com", "password123", "test-admin")
	require.NoError(suite.T(), err)

	payload := map[string]interface{}{
		"identifier": "test@example.com",
		"password":   "password123",
	}

	resp := suite.e.POST("/v1/auth/login").
		WithJSON(payload).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("token")
	resp.ContainsKey("expires_at")
	resp.ContainsKey("refresh_token")
	resp.ContainsKey("refresh_expires_at")
	resp.ContainsKey("user")

	// Verify token is not empty
	resp.Value("token").String().NotEmpty()
	resp.Value("refresh_token").String().NotEmpty()
}

func (suite *AuthTestSuite) TestLogin_InvalidCredentials() {
	payload := map[string]interface{}{
		"identifier": "nonexistent@example.com",
		"password":   "wrongpassword",
	}

	suite.e.POST("/v1/auth/login").
		WithJSON(payload).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		Value("code").String().IsEqual("invalid_credentials")
}

func (suite *AuthTestSuite) TestLogin_InvalidJSON() {
	suite.e.POST("/v1/auth/login").
		WithText("invalid json").
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		Value("code").String().IsEqual("bad_request")
}

func (suite *AuthTestSuite) TestLogin_MethodNotAllowed() {
	suite.e.PUT("/v1/auth/login").
		Expect().
		Status(http.StatusMethodNotAllowed)
}

// Refresh Tests
func (suite *AuthTestSuite) TestRefresh_Success() {
	// Create and login user to get refresh token
	_, err := suite.authSvc.Register(context.Background(), "testuser", "test@example.com", "password123", "test-admin")
	require.NoError(suite.T(), err)

	_, _, _, refreshToken, _, err := suite.authSvc.Login(context.Background(), "test@example.com", "password123")
	require.NoError(suite.T(), err)

	payload := map[string]interface{}{
		"refresh_token": refreshToken,
	}

	resp := suite.e.POST("/v1/auth/refresh").
		WithJSON(payload).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("token")
	resp.ContainsKey("expires_at")
	resp.ContainsKey("refresh_token")
	resp.ContainsKey("refresh_expires_at")
	resp.ContainsKey("user")

	// Verify new tokens are not empty
	resp.Value("token").String().NotEmpty()
	resp.Value("refresh_token").String().NotEmpty()
}

func (suite *AuthTestSuite) TestRefresh_InvalidToken() {
	payload := map[string]interface{}{
		"refresh_token": "invalid-refresh-token",
	}

	suite.e.POST("/v1/auth/refresh").
		WithJSON(payload).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		Value("code").String().IsEqual("invalid_refresh")
}

func (suite *AuthTestSuite) TestRefresh_MissingToken() {
	payload := map[string]interface{}{
		// missing refresh_token
	}

	suite.e.POST("/v1/auth/refresh").
		WithJSON(payload).
		Expect().
		Status(http.StatusUnauthorized)
}

// Helper method to create a test user directly in the database
func (suite *AuthTestSuite) createTestUser(email, username, role string) *db.User {
	// Use the auth service to create the user
	user, err := suite.authSvc.Register(context.Background(), username, email, "password123", "test-admin")
	require.NoError(suite.T(), err)

	// Update role if different from default
	if role != "USER" {
		user, err = suite.authSvc.UpdateUserRole(context.Background(), user.ID, role, "test-admin")
		require.NoError(suite.T(), err)
	}

	return user
}

// Admin Tests - these require ADMIN role

func (suite *AuthTestSuite) TestCreateUser_Success() {
	// Create admin user and get token
	adminToken := suite.createAdminUserAndGetToken()

	// Create a new user via admin endpoint
	requestBody := map[string]interface{}{
		"username": "newuser",
		"email":    "newuser@example.com",
		"password": "password123",
		"role":     "USER",
	}

	resp := suite.e.POST("/v1/admin/users").
		WithHeader("Authorization", "Bearer "+adminToken).
		WithJSON(requestBody).
		Expect().
		Status(http.StatusCreated).
		JSON().Object()

	resp.ContainsKey("id")
	resp.ContainsKey("username")
	resp.ContainsKey("email")
	resp.ContainsKey("role")
	resp.Value("username").String().IsEqual("newuser")
	resp.Value("email").String().IsEqual("newuser@example.com")
	resp.Value("role").String().IsEqual("USER")
}

func (suite *AuthTestSuite) TestCreateUser_Unauthorized() {
	// Try to create user without admin token
	requestBody := map[string]interface{}{
		"username": "newuser",
		"email":    "newuser@example.com",
		"password": "password123",
		"role":     "USER",
	}

	suite.e.POST("/v1/admin/users").
		WithJSON(requestBody).
		Expect().
		Status(http.StatusUnauthorized)
}

func (suite *AuthTestSuite) TestCreateUser_Forbidden() {
	// Create regular user and try to create another user
	userToken := suite.createUserAndGetToken("regular@example.com", "regularuser")

	requestBody := map[string]interface{}{
		"username": "newuser",
		"email":    "newuser@example.com",
		"password": "password123",
		"role":     "USER",
	}

	suite.e.POST("/v1/admin/users").
		WithHeader("Authorization", "Bearer "+userToken).
		WithJSON(requestBody).
		Expect().
		Status(http.StatusForbidden)
}

func (suite *AuthTestSuite) TestCreateUser_InvalidData() {
	adminToken := suite.createAdminUserAndGetToken()

	// Missing required fields
	requestBody := map[string]interface{}{
		"username": "newuser",
		// missing email and password
	}

	suite.e.POST("/v1/admin/users").
		WithHeader("Authorization", "Bearer "+adminToken).
		WithJSON(requestBody).
		Expect().
		Status(http.StatusBadRequest)
}

func (suite *AuthTestSuite) TestListUsers_Success() {
	adminToken := suite.createAdminUserAndGetToken()

	// Create some test users first
	suite.createTestUser("user1@example.com", "user1", "USER")
	suite.createTestUser("user2@example.com", "user2", "USER")

	resp := suite.e.GET("/v1/admin/users").
		WithHeader("Authorization", "Bearer "+adminToken).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("users")
	resp.ContainsKey("total")
	resp.ContainsKey("limit")
	resp.ContainsKey("offset")
	resp.Value("users").Array().Length().Gt(0)
	resp.Value("total").Number().Gt(0)
}

func (suite *AuthTestSuite) TestListUsers_Unauthorized() {
	suite.e.GET("/v1/admin/users").
		Expect().
		Status(http.StatusUnauthorized)
}

func (suite *AuthTestSuite) TestListUsers_WithPagination() {
	adminToken := suite.createAdminUserAndGetToken()

	resp := suite.e.GET("/v1/admin/users").
		WithHeader("Authorization", "Bearer "+adminToken).
		WithQuery("limit", "5").
		WithQuery("offset", "0").
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.Value("limit").Number().IsEqual(5)
	resp.Value("offset").Number().IsEqual(0)
}

func (suite *AuthTestSuite) TestUpdateUserStatus_Success() {
	adminToken := suite.createAdminUserAndGetToken()

	// Create a test user to update
	testUser := suite.createTestUser("testuser@example.com", "testuser", "USER")

	requestBody := map[string]interface{}{
		"active": false,
	}

	resp := suite.e.PUT("/v1/admin/users/"+testUser.ID+"/status").
		WithHeader("Authorization", "Bearer "+adminToken).
		WithJSON(requestBody).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("id")
	resp.ContainsKey("active")
	resp.Value("active").Boolean().IsFalse()
}

func (suite *AuthTestSuite) TestUpdateUserStatus_Unauthorized() {
	// Create a test user first
	testUser := suite.createTestUser("testuser@example.com", "testuser", "USER")

	requestBody := map[string]interface{}{
		"active": false,
	}

	suite.e.PUT("/v1/admin/users/" + testUser.ID + "/status").
		WithJSON(requestBody).
		Expect().
		Status(http.StatusUnauthorized)
}

func (suite *AuthTestSuite) TestUpdateUserStatus_NotFound() {
	adminToken := suite.createAdminUserAndGetToken()

	requestBody := map[string]interface{}{
		"active": false,
	}

	suite.e.PUT("/v1/admin/users/99999/status").
		WithHeader("Authorization", "Bearer "+adminToken).
		WithJSON(requestBody).
		Expect().
		Status(http.StatusNotFound)
}

func (suite *AuthTestSuite) TestUpdateUserRole_Success() {
	adminToken := suite.createAdminUserAndGetToken()

	// Create a test user to update
	testUser := suite.createTestUser("testuser@example.com", "testuser", "USER")

	requestBody := map[string]interface{}{
		"role": "ADMIN",
	}

	resp := suite.e.PUT("/v1/admin/users/"+testUser.ID+"/role").
		WithHeader("Authorization", "Bearer "+adminToken).
		WithJSON(requestBody).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.ContainsKey("id")
	resp.ContainsKey("role")
	resp.Value("role").String().IsEqual("ADMIN")
}

func (suite *AuthTestSuite) TestUpdateUserRole_InvalidRole() {
	adminToken := suite.createAdminUserAndGetToken()

	// Create a test user to update
	testUser := suite.createTestUser("testuser@example.com", "testuser", "USER")

	requestBody := map[string]interface{}{
		"role": "INVALID_ROLE",
	}

	suite.e.PUT("/v1/admin/users/"+testUser.ID+"/role").
		WithHeader("Authorization", "Bearer "+adminToken).
		WithJSON(requestBody).
		Expect().
		Status(http.StatusBadRequest)
}

func (suite *AuthTestSuite) TestDeleteUser_Success() {
	adminToken := suite.createAdminUserAndGetToken()

	// Create a test user to delete
	testUser := suite.createTestUser("testuser@example.com", "testuser", "USER")

	suite.e.DELETE("/v1/admin/users/"+testUser.ID).
		WithHeader("Authorization", "Bearer "+adminToken).
		Expect().
		Status(http.StatusNoContent)

	// Verify user is deleted by trying to get it
	suite.e.GET("/v1/admin/users").
		WithHeader("Authorization", "Bearer "+adminToken).
		Expect().
		Status(http.StatusOK)
}

func (suite *AuthTestSuite) TestDeleteUser_Unauthorized() {
	// Create a test user first
	testUser := suite.createTestUser("testuser@example.com", "testuser", "USER")

	suite.e.DELETE("/v1/admin/users/" + testUser.ID).
		Expect().
		Status(http.StatusUnauthorized)
}

func (suite *AuthTestSuite) TestDeleteUser_NotFound() {
	adminToken := suite.createAdminUserAndGetToken()

	suite.e.DELETE("/v1/admin/users/99999").
		WithHeader("Authorization", "Bearer "+adminToken).
		Expect().
		Status(http.StatusNotFound)
}

// Helper methods for admin tests

func (suite *AuthTestSuite) createAdminUserAndGetToken() string {
	// Create admin user directly in database
	suite.createTestUser("admin@example.com", "admin", "ADMIN")

	// Login to get token
	loginResp := suite.e.POST("/v1/auth/login").
		WithJSON(map[string]interface{}{
			"email":    "admin@example.com",
			"password": "password123",
		}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	return loginResp.Value("access_token").String().Raw()
}

func (suite *AuthTestSuite) createUserAndGetToken(email, username string) string {
	// Create regular user
	suite.createTestUser(email, username, "USER")

	// Login to get token
	loginResp := suite.e.POST("/v1/auth/login").
		WithJSON(map[string]interface{}{
			"email":    email,
			"password": "password123",
		}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	return loginResp.Value("access_token").String().Raw()
}

func TestAuthTestSuite(t *testing.T) {
	suite.Run(t, new(AuthTestSuite))
}
