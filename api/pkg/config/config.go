// Package config loads and validates the API's runtime configuration.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"go.uber.org/zap/zapcore"
)

// Config contains the validated runtime settings for every API subsystem.
type Config struct {
	App      AppConfig
	Server   ServerConfig
	Database DatabaseConfig
	Auth     AuthConfig
	OpenAI   OpenAIConfig
	DeepL    DeepLConfig
	Telegram TelegramConfig
	Prompts  PromptConfig
	Limits   LimitConfig
}

// AppConfig controls environment-specific process behavior and logging.
type AppConfig struct {
	Version     string `env:"APP_VERSION"`
	Environment string `env:"APP_ENV" env-default:"development"`
	LogLevel    string `env:"LOG_LEVEL" env-default:"info"`
}

// ServerConfig controls HTTP lifecycle, request limits, CORS, and rate limiting.
type ServerConfig struct {
	Port              string        `env:"PORT" env-default:"4161"`
	CORSOrigins       []string      `env:"CORS_ORIGINS" env-separator:"," env-default:"http://localhost:5173"`
	ReadHeaderTimeout time.Duration `env:"HTTP_READ_HEADER_TIMEOUT" env-default:"10s"`
	ReadTimeout       time.Duration `env:"HTTP_READ_TIMEOUT" env-default:"30s"`
	RequestTimeout    time.Duration `env:"HTTP_REQUEST_TIMEOUT" env-default:"2m"`
	WriteTimeout      time.Duration `env:"HTTP_WRITE_TIMEOUT" env-default:"2m5s"`
	IdleTimeout       time.Duration `env:"HTTP_IDLE_TIMEOUT" env-default:"2m"`
	GracefulTimeout   time.Duration `env:"HTTP_GRACEFUL_TIMEOUT" env-default:"10s"`
	MaxBodyBytes      int64         `env:"HTTP_MAX_BODY_BYTES" env-default:"131072"`
	RateLimit         float64       `env:"HTTP_RATE_LIMIT" env-default:"10"`
	RateBurst         int           `env:"HTTP_RATE_BURST" env-default:"20"`
	RateLimiterExpiry time.Duration `env:"HTTP_RATE_LIMITER_EXPIRY" env-default:"3m"`
}

// DatabaseConfig controls PostgreSQL connectivity and pool behavior.
type DatabaseConfig struct {
	URL                   string        `env:"DATABASE_URL"`
	URLFile               string        `env:"DATABASE_URL_FILE"`
	ApplicationName       string        `env:"DATABASE_APPLICATION_NAME" env-default:"ai_translate_api"`
	MaxConnections        int32         `env:"DATABASE_MAX_CONNECTIONS" env-default:"10"`
	MinConnections        int32         `env:"DATABASE_MIN_CONNECTIONS" env-default:"0"`
	MinIdleConnections    int32         `env:"DATABASE_MIN_IDLE_CONNECTIONS" env-default:"1"`
	MaxConnectionLifetime time.Duration `env:"DATABASE_MAX_CONNECTION_LIFETIME" env-default:"1h"`
	LifetimeJitter        time.Duration `env:"DATABASE_LIFETIME_JITTER" env-default:"5m"`
	MaxConnectionIdleTime time.Duration `env:"DATABASE_MAX_CONNECTION_IDLE_TIME" env-default:"15m"`
	HealthCheckPeriod     time.Duration `env:"DATABASE_HEALTH_CHECK_PERIOD" env-default:"1m"`
	PingTimeout           time.Duration `env:"DATABASE_PING_TIMEOUT" env-default:"5s"`
	ConnectTimeout        time.Duration `env:"DATABASE_CONNECT_TIMEOUT" env-default:"5s"`
	StatementTimeout      time.Duration `env:"DATABASE_STATEMENT_TIMEOUT" env-default:"5s"`
}

// AuthConfig controls token signing and browser cookie security.
type AuthConfig struct {
	JWTSecret     string `env:"JWT_SECRET"`
	JWTSecretFile string `env:"JWT_SECRET_FILE"`
	SecureCookies bool   `env:"AUTH_SECURE_COOKIES" env-default:"false"`
}

// OpenAIConfig controls model selection and provider request behavior.
type OpenAIConfig struct {
	APIKey               string        `env:"OPENAI_API_KEY"`
	APIKeyFile           string        `env:"OPENAI_API_KEY_FILE"`
	Model                string        `env:"OPENAI_MODEL" env-default:"gpt-4.1-nano"`
	TranslationLanguages []string      `env:"OPENAI_TRANSLATION_LANGUAGES" env-separator:","`
	RequestTimeout       time.Duration `env:"OPENAI_REQUEST_TIMEOUT" env-default:"90s"`
	MaxRetries           int           `env:"OPENAI_MAX_RETRIES" env-default:"2"`
}

// DeepLConfig supplies one or more provider credentials, inline or by file.
type DeepLConfig struct {
	APIKeys     []string `env:"DEEPL_API_KEYS" env-separator:","`
	APIKeysFile string   `env:"DEEPL_API_KEYS_FILE"`
}

// TelegramConfig controls delivery of panic alerts through a Telegram bot.
type TelegramConfig struct {
	BotAPIKey      string        `env:"TELEGRAM_BOT_API_KEY"`
	BotAPIKeyFile  string        `env:"TELEGRAM_BOT_API_KEY_FILE"`
	ChatID         int64         `env:"TELEGRAM_CHAT_ID"`
	RequestTimeout time.Duration `env:"TELEGRAM_REQUEST_TIMEOUT" env-default:"5s"`
}

// PromptConfig supplies the model instructions used by each AI operation.
type PromptConfig struct {
	Translate     string `env:"TRANSLATE_PROMPT"`
	TranslateFile string `env:"TRANSLATE_PROMPT_FILE"`
	Interpret     string `env:"INTERPRET_PROMPT"`
	InterpretFile string `env:"INTERPRET_PROMPT_FILE"`
	Analyze       string `env:"ANALYZE_PROMPT"`
	AnalyzeFile   string `env:"ANALYZE_PROMPT_FILE"`
}

// LimitConfig defines request-size constraints enforced by feature handlers.
type LimitConfig struct {
	TranslationCharacters int `env:"MAX_TRANSLATION_CHARACTERS" env-default:"10000"`
}

// Load reads environment variables, resolves values supplied through files,
// and rejects unsafe or internally inconsistent settings.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := cleanenv.ReadEnv(cfg); err != nil {
		return nil, fmt.Errorf("read environment: %w", err)
	}

	var err error
	if cfg.Database.URL, err = resolveValue("DATABASE_URL", cfg.Database.URL, cfg.Database.URLFile); err != nil {
		return nil, err
	}
	if cfg.Auth.JWTSecret, err = resolveValue("JWT_SECRET", cfg.Auth.JWTSecret, cfg.Auth.JWTSecretFile); err != nil {
		return nil, err
	}
	if cfg.OpenAI.APIKey, err = resolveValue("OPENAI_API_KEY", cfg.OpenAI.APIKey, cfg.OpenAI.APIKeyFile); err != nil {
		return nil, err
	}
	if cfg.Telegram.BotAPIKey, err = resolveValue("TELEGRAM_BOT_API_KEY", cfg.Telegram.BotAPIKey, cfg.Telegram.BotAPIKeyFile); err != nil {
		return nil, err
	}
	if cfg.Prompts.Translate, err = resolveValue("TRANSLATE_PROMPT", cfg.Prompts.Translate, cfg.Prompts.TranslateFile); err != nil {
		return nil, err
	}
	if cfg.Prompts.Interpret, err = resolveValue("INTERPRET_PROMPT", cfg.Prompts.Interpret, cfg.Prompts.InterpretFile); err != nil {
		return nil, err
	}
	if cfg.Prompts.Analyze, err = resolveValue("ANALYZE_PROMPT", cfg.Prompts.Analyze, cfg.Prompts.AnalyzeFile); err != nil {
		return nil, err
	}
	if len(cfg.DeepL.APIKeys) > 0 && cfg.DeepL.APIKeysFile != "" {
		return nil, errors.New("set only one of DEEPL_API_KEYS and DEEPL_API_KEYS_FILE")
	}
	if cfg.DeepL.APIKeysFile != "" {
		contents, readErr := os.ReadFile(cfg.DeepL.APIKeysFile)
		if readErr != nil {
			return nil, fmt.Errorf("read DEEPL_API_KEYS_FILE: %w", readErr)
		}
		cfg.DeepL.APIKeys = splitList(string(contents))
	}
	cfg.DeepL.APIKeys = cleanList(cfg.DeepL.APIKeys)
	cfg.Server.CORSOrigins = cleanList(cfg.Server.CORSOrigins)
	cfg.OpenAI.TranslationLanguages = cleanList(cfg.OpenAI.TranslationLanguages)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate configuration: %w", err)
	}
	return cfg, nil
}

// Validate rejects missing, unsafe, and internally inconsistent settings.
func (cfg *Config) Validate() error {
	if cfg == nil {
		return errors.New("configuration is nil")
	}

	var level zapcore.Level
	if err := level.UnmarshalText([]byte(strings.ToLower(strings.TrimSpace(cfg.App.LogLevel)))); err != nil {
		return fmt.Errorf("LOG_LEVEL: %w", err)
	}
	switch cfg.App.Environment {
	case "development", "test", "staging", "production":
	default:
		return fmt.Errorf("APP_ENV must be development, test, staging, or production, got %q", cfg.App.Environment)
	}

	port, err := strconv.Atoi(cfg.Server.Port)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("PORT must be a number from 1 to 65535, got %q", cfg.Server.Port)
	}
	if len(cfg.Server.CORSOrigins) == 0 {
		return errors.New("CORS_ORIGINS must contain at least one origin")
	}
	for _, origin := range cfg.Server.CORSOrigins {
		if err := validateOrigin(origin); err != nil {
			return fmt.Errorf("CORS_ORIGINS: %w", err)
		}
	}
	if err := validateServer(cfg.Server); err != nil {
		return err
	}
	if err := validateDatabase(cfg.Database); err != nil {
		return err
	}
	if len(cfg.Auth.JWTSecret) < 32 {
		return errors.New("JWT_SECRET must contain at least 32 characters")
	}
	if cfg.App.Environment == "production" && !cfg.Auth.SecureCookies {
		return errors.New("AUTH_SECURE_COOKIES must be true in production")
	}
	if strings.TrimSpace(cfg.OpenAI.APIKey) == "" {
		return errors.New("OPENAI_API_KEY or OPENAI_API_KEY_FILE is required")
	}
	if strings.TrimSpace(cfg.OpenAI.Model) == "" {
		return errors.New("OPENAI_MODEL must not be empty")
	}
	if err := validateUniqueList("OPENAI_TRANSLATION_LANGUAGES", cfg.OpenAI.TranslationLanguages); err != nil {
		return err
	}
	if cfg.OpenAI.RequestTimeout <= 0 {
		return errors.New("OPENAI_REQUEST_TIMEOUT must be positive")
	}
	if cfg.OpenAI.RequestTimeout >= cfg.Server.RequestTimeout {
		return errors.New("OPENAI_REQUEST_TIMEOUT must be shorter than HTTP_REQUEST_TIMEOUT")
	}
	if cfg.OpenAI.MaxRetries < 0 {
		return errors.New("OPENAI_MAX_RETRIES must not be negative")
	}
	if len(cfg.DeepL.APIKeys) == 0 {
		return errors.New("DEEPL_API_KEYS or DEEPL_API_KEYS_FILE is required")
	}
	telegramKeySet := strings.TrimSpace(cfg.Telegram.BotAPIKey) != ""
	telegramChatSet := cfg.Telegram.ChatID != 0
	if telegramKeySet != telegramChatSet {
		return errors.New("TELEGRAM_BOT_API_KEY and TELEGRAM_CHAT_ID must be set together")
	}
	if cfg.App.Environment == "production" && !telegramKeySet {
		return errors.New("TELEGRAM_BOT_API_KEY or TELEGRAM_BOT_API_KEY_FILE and TELEGRAM_CHAT_ID are required in production")
	}
	if cfg.Telegram.RequestTimeout <= 0 {
		return errors.New("TELEGRAM_REQUEST_TIMEOUT must be positive")
	}
	if strings.TrimSpace(cfg.Prompts.Translate) == "" {
		return errors.New("TRANSLATE_PROMPT or TRANSLATE_PROMPT_FILE is required")
	}
	if strings.TrimSpace(cfg.Prompts.Interpret) == "" {
		return errors.New("INTERPRET_PROMPT or INTERPRET_PROMPT_FILE is required")
	}
	if strings.TrimSpace(cfg.Prompts.Analyze) == "" {
		return errors.New("ANALYZE_PROMPT or ANALYZE_PROMPT_FILE is required")
	}
	if cfg.Limits.TranslationCharacters <= 0 {
		return errors.New("MAX_TRANSLATION_CHARACTERS must be positive")
	}
	return nil
}

func validateServer(cfg ServerConfig) error {
	if cfg.ReadHeaderTimeout <= 0 || cfg.ReadTimeout <= 0 || cfg.RequestTimeout <= 0 ||
		cfg.WriteTimeout <= 0 || cfg.IdleTimeout <= 0 || cfg.GracefulTimeout <= 0 {
		return errors.New("all HTTP timeouts must be positive")
	}
	if cfg.RequestTimeout >= cfg.WriteTimeout {
		return errors.New("HTTP_REQUEST_TIMEOUT must be shorter than HTTP_WRITE_TIMEOUT")
	}
	if cfg.MaxBodyBytes <= 0 {
		return errors.New("HTTP_MAX_BODY_BYTES must be positive")
	}
	if cfg.RateLimit <= 0 || cfg.RateBurst <= 0 || cfg.RateLimiterExpiry <= 0 {
		return errors.New("HTTP rate-limit settings must be positive")
	}
	return nil
}

func validateDatabase(cfg DatabaseConfig) error {
	if strings.TrimSpace(cfg.URL) == "" {
		return errors.New("DATABASE_URL or DATABASE_URL_FILE is required")
	}
	if strings.TrimSpace(cfg.ApplicationName) == "" {
		return errors.New("DATABASE_APPLICATION_NAME must not be empty")
	}
	if cfg.MaxConnections <= 0 || cfg.MinConnections < 0 || cfg.MinIdleConnections < 0 {
		return errors.New("database connection counts are invalid")
	}
	if cfg.MinConnections > cfg.MaxConnections || cfg.MinIdleConnections > cfg.MaxConnections {
		return errors.New("database minimum connections cannot exceed DATABASE_MAX_CONNECTIONS")
	}
	if cfg.MaxConnectionLifetime <= 0 || cfg.LifetimeJitter < 0 || cfg.MaxConnectionIdleTime <= 0 ||
		cfg.HealthCheckPeriod <= 0 || cfg.PingTimeout <= 0 || cfg.ConnectTimeout <= 0 || cfg.StatementTimeout <= 0 {
		return errors.New("database durations must be positive (lifetime jitter may be zero)")
	}
	return nil
}

func validateOrigin(origin string) error {
	origin = strings.TrimSpace(origin)
	if origin == "*" {
		return errors.New("wildcard origin is not allowed")
	}
	u, err := url.Parse(origin)
	if err != nil || !isAllowedOriginScheme(u.Scheme) || u.Host == "" {
		return fmt.Errorf("invalid origin %q", origin)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("origin must not include credentials, a path, query, or fragment: %q", origin)
	}
	return nil
}

func isAllowedOriginScheme(scheme string) bool {
	switch scheme {
	case "http", "https", "chrome-extension", "moz-extension":
		return true
	default:
		return false
	}
}

func resolveValue(name, value, file string) (string, error) {
	value = strings.TrimSpace(value)
	file = strings.TrimSpace(file)
	if value != "" && file != "" {
		return "", fmt.Errorf("set only one of %s and %s_FILE", name, name)
	}
	if file == "" {
		return value, nil
	}
	contents, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", name, err)
	}
	return strings.TrimSpace(string(contents)), nil
}

func splitList(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
}

func cleanList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func validateUniqueList(name string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s must contain at least one value", name)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%s contains duplicate value %q", name, value)
		}
		seen[key] = struct{}{}
	}
	return nil
}
