package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"go-demo/internal/config"
	"go-demo/internal/sqllog"

	"github.com/sashabaranov/go-openai"
)

// AIAnalysisHandler handles AI-powered SQL analysis
type AIAnalysisHandler struct {
	repo   *sqllog.Repository
	log    *slog.Logger
	client *openai.Client
}

// NewAIAnalysisHandler creates a new AI analysis handler
func NewAIAnalysisHandler(repo *sqllog.Repository, log *slog.Logger, cfg config.Config) *AIAnalysisHandler {
	if log == nil {
		log = slog.Default()
	}

	var client *openai.Client
	if cfg.OpenAIAPIKey != "" {
		client = openai.NewClient(cfg.OpenAIAPIKey)
	}

	return &AIAnalysisHandler{
		repo:   repo,
		log:    log,
		client: client,
	}
}

// AnalysisResult represents the response structure
type AnalysisResult struct {
	Status string          `json:"status"`
	Data   []QueryAnalysis `json:"data,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// QueryAnalysis represents analysis of a single query
type QueryAnalysis struct {
	ID          uint64 `json:"id"`
	SQLQuery    string `json:"sql_query"`
	ExecTimeMs  int64  `json:"exec_time_ms"`
	ExecCount   int64  `json:"exec_count"`
	Suggestions string `json:"suggestions"`
}

// AIAnalysis godoc
// @Summary AI analysis endpoint
// @Tags ai
// @Param db_name query string true "Database name"
// @Success 200 {object} AnalysisResult
// @Failure 400 {object} AnalysisResult
// @Failure 500 {object} AnalysisResult
// @Router /v1/ai-analysis [get]
func (h *AIAnalysisHandler) AIAnalysis() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbName := r.URL.Query().Get("db_name")
		if dbName == "" {
			h.writeErrorResponse(w, http.StatusBadRequest, "db_name parameter is required")
			return
		}

		// Query slow queries from database
		queries, err := h.repo.FindSlowQueries(r.Context(), dbName)
		if err != nil {
			h.log.Error("Failed to query slow queries", "error", err, "db_name", dbName)
			h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to query database")
			return
		}

		if len(queries) == 0 {
			h.writeSuccessResponse(w, []QueryAnalysis{})
			return
		}

		// Analyze queries with AI
		analyses := make([]QueryAnalysis, len(queries))
		for i, query := range queries {
			suggestions, err := h.analyzeQueryWithAI(r.Context(), query.SQLQuery)
			if err != nil {
				h.log.Error("Failed to analyze query with AI", "error", err, "query_id", query.ID)
				suggestions = "Recommendation: manual review required"
			}

			analyses[i] = QueryAnalysis{
				ID:          query.ID,
				SQLQuery:    query.SQLQuery,
				ExecTimeMs:  query.ExecTimeMs,
				ExecCount:   query.ExecCount,
				Suggestions: suggestions,
			}
		}

		h.writeSuccessResponse(w, analyses)
	}
}

// analyzeQueryWithAI uses OpenAI to analyze SQL queries and provide optimization suggestions
func (h *AIAnalysisHandler) analyzeQueryWithAI(ctx context.Context, sqlQuery string) (string, error) {
	if h.client == nil {
		return h.analyzeQueryLocally(sqlQuery), nil
	}

	prompt := fmt.Sprintf(`You are a database optimization assistant.
Your task is to analyze unusual SQL queries and provide optimization suggestions based on the following rules:
When an SQL query is detected, analyze the WHERE clause to identify the fields used.
If the WHERE clause contains a single field, suggest: "Add index on [field_name]".
If the WHERE clause has multiple fields, suggest indexes for all relevant fields.
Continue analysis the query to identify potential performance improvements.
If the query cannot be analyzed to provide suggestions, return: "Recommendation: manual review required".
Apply these rules to any SQL statement I provide.

Query to analyze:
%s`, sqlQuery)

	resp, err := h.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4Dot1Nano,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		MaxTokens:   16 * 1024,
		Temperature: 0.1,
	})

	if err != nil {
		return "", fmt.Errorf("OpenAI API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// analyzeQueryLocally provides basic local analysis as fallback
func (h *AIAnalysisHandler) analyzeQueryLocally(sqlQuery string) string {
	// Basic regex to extract WHERE clause fields
	whereRegex := regexp.MustCompile(`(?i)WHERE\s+(.+?)(?:\s+ORDER\s+BY|\s+GROUP\s+BY|\s+HAVING|\s+LIMIT|$)`)
	matches := whereRegex.FindStringSubmatch(sqlQuery)

	if len(matches) < 2 {
		return "Recommendation: manual review required"
	}

	whereClause := matches[1]

	// Extract field names (basic approach)
	fieldRegex := regexp.MustCompile(`(\w+)\s*[=<>!]`)
	fieldMatches := fieldRegex.FindAllStringSubmatch(whereClause, -1)

	if len(fieldMatches) == 0 {
		return "Recommendation: manual review required"
	}

	var fields []string
	for _, match := range fieldMatches {
		if len(match) > 1 {
			fields = append(fields, match[1])
		}
	}

	if len(fields) == 1 {
		return fmt.Sprintf("Add index on %s", fields[0])
	} else if len(fields) > 1 {
		return fmt.Sprintf("Add indexes on %s", strings.Join(fields, ", "))
	}

	return "Recommendation: manual review required"
}

func (h *AIAnalysisHandler) writeSuccessResponse(w http.ResponseWriter, data []QueryAnalysis) {
	response := AnalysisResult{
		Status: "success",
		Data:   data,
	}
	h.writeJSONResponse(w, http.StatusOK, response)
}

func (h *AIAnalysisHandler) writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	response := AnalysisResult{
		Status: "error",
		Error:  message,
	}
	h.writeJSONResponse(w, statusCode, response)
}

func (h *AIAnalysisHandler) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.log.Error("Failed to encode JSON response", "error", err)
	}
}
