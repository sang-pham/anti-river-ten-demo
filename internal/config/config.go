package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port           string
	LogLevel       string
	RequestTimeout time.Duration
	MaxBodyBytes   int64
	AllowedOrigins []string
	Env            string

	// Auth/DB
	DatabaseURL string
	JWTSecret   string
	JWTTTL      time.Duration
	RefreshTTL  time.Duration

	// OpenAI
	OpenAIAPIKey string
}

func FromEnv() (Config, error) {
	cfg := Config{
		Port:           getenv("PORT", "8080"),
		LogLevel:       getenv("LOG_LEVEL", "info"),
		RequestTimeout: parseDuration(getenv("REQUEST_TIMEOUT", "30s"), 30*time.Second),
		MaxBodyBytes:   parseInt64(getenv("MAX_BODY_BYTES", "1048576"), 1048576),
		AllowedOrigins: parseCSV(getenv("ALLOWED_ORIGINS", "")),
		Env:            getenv("APP_ENV", "development"),

		DatabaseURL: getenv("DATABASE_URL", ""),
		JWTSecret:   getenv("JWT_SECRET", ""),
		JWTTTL:      parseDuration(getenv("JWT_TTL", "24h"), 24*time.Hour),
		RefreshTTL:  parseDuration(getenv("REFRESH_TTL", "720h"), 720*time.Hour), // 30 days

		OpenAIAPIKey: getenv("OPENAI_API_KEY", ""),
	}

	// Default to permissive CORS in non-production if not explicitly configured.
	// This prevents local dev CORS errors when ALLOWED_ORIGINS is omitted.
	if len(cfg.AllowedOrigins) == 0 && cfg.Env != "production" {
		cfg.AllowedOrigins = []string{"*"}
	}

	return cfg, nil
}

func getenv(k, def string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return def
}

func parseDuration(s string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return def
}

func parseInt64(s string, def int64) int64 {
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v
	}
	return def
}

func parseCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
