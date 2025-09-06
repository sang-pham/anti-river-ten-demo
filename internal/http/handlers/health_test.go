package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gavv/httpexpect/v2"
	"github.com/stretchr/testify/suite"
)

type HealthTestSuite struct {
	suite.Suite
	e      *httpexpect.Expect
	server *httptest.Server
}

func (suite *HealthTestSuite) SetupSuite() {
	// Create a simple test server with just the health handlers
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", Healthz)
	mux.HandleFunc("GET /readyz", Readyz)

	suite.server = httptest.NewServer(mux)
	suite.e = httpexpect.Default(suite.T(), suite.server.URL)
}

func (suite *HealthTestSuite) TearDownSuite() {
	suite.server.Close()
}

func (suite *HealthTestSuite) TestHealthz_Success() {
	suite.e.GET("/healthz").
		Expect().
		Status(http.StatusOK).
		Text().IsEqual("ok")
}

func (suite *HealthTestSuite) TestHealthz_WrongMethod() {
	suite.e.POST("/healthz").
		Expect().
		Status(http.StatusMethodNotAllowed)
}

func (suite *HealthTestSuite) TestReadyz_Success() {
	suite.e.GET("/readyz").
		Expect().
		Status(http.StatusOK).
		Text().IsEqual("ready")
}

func (suite *HealthTestSuite) TestReadyz_WrongMethod() {
	suite.e.PUT("/readyz").
		Expect().
		Status(http.StatusMethodNotAllowed)
}

func TestHealthTestSuite(t *testing.T) {
	suite.Run(t, new(HealthTestSuite))
}
