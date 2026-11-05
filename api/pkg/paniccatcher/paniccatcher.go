// Package paniccatcher provides HTTP panic recovery with structured logging
// and Telegram alerting.
package paniccatcher

import (
	"crypto/sha256"
	"fmt"
	"html"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/bgdimetrius/AITranslate/pkg"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

const (
	telegramBotContextKey    = "paniccatcher.telegram_bot"
	telegramChatIDContextKey = "paniccatcher.telegram_chat_id"
	maxPanicMessageRunes     = 800
)

// TelegramSender is the Telegram operation required to deliver a panic alert.
type TelegramSender interface {
	Send(tgbotapi.Chattable) (tgbotapi.Message, error)
}

// SetTelegramAlertTarget stores the bot and destination chat on the request
// context so Middleware can alert without relying on package-global state.
func SetTelegramAlertTarget(c *echo.Context, bot TelegramSender, chatID int64) {
	c.Set(telegramBotContextKey, bot)
	c.Set(telegramChatIDContextKey, chatID)
}

// Middleware recovers panics raised by handlers and middleware in the current
// request goroutine. It logs the full stack, attempts one Telegram alert, and
// returns a generic 500 response without exposing internal details to clients.
// The alert includes a stable incident fingerprint, request ID, HTTP method,
// registered route path, and response state; it excludes the query string and
// points operators to the application logs for the full stack trace.
// Telegram delivery failures are logged but never replace the recovered error.
// It re-panics http.ErrAbortHandler to preserve net/http's aborted-request
// behavior.
//
// Panics raised by goroutines started inside a handler must be recovered inside
// those goroutines; a parent request goroutine cannot recover them.
func Middleware(fallbackLog *zap.Logger) echo.MiddlewareFunc {
	if fallbackLog == nil {
		fallbackLog = zap.NewNop()
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) (err error) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}

				// net/http uses this sentinel to suppress its own stack trace when a
				// client connection is deliberately aborted.
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}

				panicErr := recoveredError(recovered)
				requestLog := fallbackLog
				if scopedLog, ok := c.Get("logger").(*zap.Logger); ok && scopedLog != nil {
					requestLog = scopedLog
				}

				method, path := requestRoute(c)
				committed := responseCommitted(c)
				stack := debug.Stack()

				requestLog.Error("panic recovered",
					zap.Error(panicErr),
					zap.String("method", method),
					zap.String("path", path),
					zap.Bool("response_committed", committed),
					zap.String("stack_trace", string(stack)),
				)

				// Alerting is best effort because a Telegram outage must not prevent
				// Echo from completing panic recovery.
				if alertErr := sendTelegramAlert(c, panicErr, method, path, committed); alertErr != nil {
					requestLog.Error("panic alert delivery failed", zap.Error(alertErr))
				}

				err = echo.NewHTTPError(http.StatusInternalServerError, pkg.SmthWW).Wrap(panicErr)
			}()

			return next(c)
		}
	}
}

func requestRoute(c *echo.Context) (method string, path string) {
	path = c.Path()
	if request := c.Request(); request != nil {
		method = request.Method
		if path == "" && request.URL != nil {
			path = request.URL.Path
		}
	}
	return method, path
}

func responseCommitted(c *echo.Context) bool {
	response, err := echo.UnwrapResponse(c.Response())
	return err == nil && response.Committed
}

func sendTelegramAlert(c *echo.Context, panicErr error, method string, path string, committed bool) (err error) {
	// A faulty alerting dependency runs inside recovery itself, so contain any
	// secondary panic here instead of letting it escape the HTTP server.
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("telegram sender panicked: %v", recovered)
		}
	}()

	bot, ok := c.Get(telegramBotContextKey).(TelegramSender)
	if !ok || bot == nil {
		return fmt.Errorf("telegram bot is missing from request context")
	}
	chatID, ok := c.Get(telegramChatIDContextKey).(int64)
	if !ok || chatID == 0 {
		return fmt.Errorf("telegram chat ID is missing from request context")
	}

	message := tgbotapi.NewMessage(chatID, formatAlert(c, panicErr, method, path, committed))
	message.ParseMode = tgbotapi.ModeHTML
	message.DisableWebPagePreview = true
	_, err = bot.Send(message)
	return err
}

func formatAlert(c *echo.Context, panicErr error, method string, path string, committed bool) string {
	requestID := c.Response().Header().Get(echo.HeaderXRequestID)
	if requestID == "" {
		requestID = "unavailable"
	}
	if method == "" {
		method = "unknown"
	}
	if path == "" {
		path = "unknown"
	}

	fingerprintSource := method + "\x00" + path + "\x00" + panicErr.Error()
	fingerprint := sha256.Sum256([]byte(fingerprintSource))
	responseStarted := "no"
	if committed {
		responseStarted = "yes"
	}

	return fmt.Sprintf(
		"🚨 <b>AI Translate API caught a panic</b>\n\n"+
			"<b>Incident:</b> <code>%X</code>\n"+
			"<b>Request:</b> <code>%s %s</code>\n"+
			"<b>Request ID:</b> <code>%s</code>\n"+
			"<b>Response already started:</b> <code>%s</code>\n"+
			"<b>Detected:</b> <code>%s</code>\n\n"+
			"<b>Panic</b>\n<pre>%s</pre>\n"+
			"Full stack trace is preserved in the application logs.",
		fingerprint[:4],
		escapeAndLimit(method, 32),
		escapeAndLimit(path, 512),
		escapeAndLimit(requestID, 128),
		responseStarted,
		time.Now().UTC().Format(time.RFC3339),
		escapeAndLimit(panicErr.Error(), maxPanicMessageRunes),
	)
}

func escapeAndLimit(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > limit {
		runes = append(runes[:limit], '…')
	}
	return html.EscapeString(string(runes))
}

func recoveredError(value any) error {
	if err, ok := value.(error); ok {
		return err
	}
	return fmt.Errorf("%v", value)
}
