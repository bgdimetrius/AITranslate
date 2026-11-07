package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The provider catalog depends on preserving every configured language from
// the comma-separated environment value.
func TestLoadReadsOpenAITranslationLanguages(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("OPENAI_TRANSLATION_LANGUAGES", "en, de,ka")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := []string{"en", "de", "ka"}
	if !reflect.DeepEqual(cfg.OpenAI.TranslationLanguages, want) {
		t.Fatalf("TranslationLanguages = %v; want %v", cfg.OpenAI.TranslationLanguages, want)
	}
}

// Production must not start without panic alerts, and partial alert targets
// are invalid in every environment.
func TestLoadValidatesTelegramConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		secure      string
		botAPIKey   string
		chatID      string
		timeout     string
		wantError   string
	}{
		{name: "alerts may be disabled outside production", environment: "test", secure: "false", chatID: "0", timeout: "5s"},
		{name: "configured outside production", environment: "test", secure: "false", botAPIKey: "123:test-token", chatID: "42", timeout: "5s"},
		{name: "bot key without chat", environment: "test", secure: "false", botAPIKey: "123:test-token", chatID: "0", timeout: "5s", wantError: "must be set together"},
		{name: "chat without bot key", environment: "test", secure: "false", chatID: "42", timeout: "5s", wantError: "must be set together"},
		{name: "production requires alerts", environment: "production", secure: "true", chatID: "0", timeout: "5s", wantError: "required in production"},
		{name: "production with alerts", environment: "production", secure: "true", botAPIKey: "123:test-token", chatID: "-100123", timeout: "5s"},
		{name: "timeout must be positive", environment: "test", secure: "false", chatID: "0", timeout: "0s", wantError: "TELEGRAM_REQUEST_TIMEOUT must be positive"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv("APP_ENV", test.environment)
			t.Setenv("AUTH_SECURE_COOKIES", test.secure)
			t.Setenv("TELEGRAM_BOT_API_KEY", test.botAPIKey)
			t.Setenv("TELEGRAM_CHAT_ID", test.chatID)
			t.Setenv("TELEGRAM_REQUEST_TIMEOUT", test.timeout)

			_, err := Load()
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("Load() error = %v; want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Load() error = %v; want error containing %q", err, test.wantError)
			}
		})
	}
}

// Production reads the bot credential from a Docker secret file so it need not
// be exposed through a container environment value.
func TestLoadReadsTelegramBotAPIKeyFile(t *testing.T) {
	setValidEnvironment(t)
	secretFile := filepath.Join(t.TempDir(), "telegram_bot_api_key")
	if err := os.WriteFile(secretFile, []byte("123:file-token\n"), 0o600); err != nil {
		t.Fatalf("write Telegram bot key file: %v", err)
	}
	t.Setenv("TELEGRAM_BOT_API_KEY", "")
	t.Setenv("TELEGRAM_BOT_API_KEY_FILE", secretFile)
	t.Setenv("TELEGRAM_CHAT_ID", "-100123")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Telegram.BotAPIKey != "123:file-token" {
		t.Fatalf("BotAPIKey = %q; want value read from secret file", cfg.Telegram.BotAPIKey)
	}
}

func setValidEnvironment(t *testing.T) {
	t.Helper()

	environment := map[string]string{
		"APP_ENV":                      "test",
		"DATABASE_URL":                 "postgres://postgres:postgres@localhost:5432/ai_translate?sslmode=disable",
		"DATABASE_URL_FILE":            "",
		"JWT_SECRET":                   "01234567890123456789012345678901",
		"JWT_SECRET_FILE":              "",
		"OPENAI_API_KEY":               "test-key",
		"OPENAI_API_KEY_FILE":          "",
		"OPENAI_TRANSLATION_LANGUAGES": "en,de,ka",
		"DEEPL_API_KEYS":               "test-key",
		"DEEPL_API_KEYS_FILE":          "",
		"TELEGRAM_BOT_API_KEY":         "",
		"TELEGRAM_BOT_API_KEY_FILE":    "",
		"TELEGRAM_CHAT_ID":             "0",
		"TELEGRAM_REQUEST_TIMEOUT":     "5s",
		"TRANSLATE_PROMPT":             "translate",
		"TRANSLATE_PROMPT_FILE":        "",
		"INTERPRET_PROMPT":             "interpret",
		"INTERPRET_PROMPT_FILE":        "",
		"ANALYZE_PROMPT":               "analyze",
		"ANALYZE_PROMPT_FILE":          "",
		"AUTH_SECURE_COOKIES":          "false",
		"OPENAI_REQUEST_TIMEOUT":       "90s",
		"HTTP_REQUEST_TIMEOUT":         "2m",
		"HTTP_WRITE_TIMEOUT":           "2m5s",
	}
	for name, value := range environment {
		t.Setenv(name, value)
	}
}
