package paniccatcher

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bgdimetrius/AITranslate/pkg"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type recordingTelegramSender struct {
	messages   []tgbotapi.Chattable
	sendErr    error
	panicValue any
}

func (s *recordingTelegramSender) Send(message tgbotapi.Chattable) (tgbotapi.Message, error) {
	if s.panicValue != nil {
		panic(s.panicValue)
	}
	s.messages = append(s.messages, message)
	return tgbotapi.Message{}, s.sendErr
}

// Panic alerts must omit request queries, and Telegram failures must not break
// the safe HTTP response.
func TestMiddleware(t *testing.T) {
	t.Run("request without panic passes through", func(t *testing.T) {
		log, observed := observedLogger()
		sender := &recordingTelegramSender{}
		c := newTestContext(http.MethodGet, "/health")
		SetTelegramAlertTarget(c, sender, 42)
		wantErr := errors.New("handler error")

		err, recovered := invoke(Middleware(log)(func(*echo.Context) error { return wantErr }), c)

		if recovered != nil {
			t.Fatalf("recovered value = %v; want nil", recovered)
		}
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v; want %v", err, wantErr)
		}
		if len(sender.messages) != 0 {
			t.Fatalf("Telegram messages = %d; want 0", len(sender.messages))
		}
		if observed.Len() != 0 {
			t.Fatalf("error logs = %d; want 0", observed.Len())
		}
	})

	t.Run("panic produces safe response and useful alert", func(t *testing.T) {
		log, observed := observedLogger()
		sender := &recordingTelegramSender{}
		c := newTestContext(http.MethodPost, "/api/v1/translate?text=private-query")
		c.SetPath("/api/v1/translate")
		c.Response().Header().Set(echo.HeaderXRequestID, "request-123")
		SetTelegramAlertTarget(c, sender, -1001234567890)

		err, recovered := invoke(Middleware(log)(func(*echo.Context) error {
			panic("<database & provider unavailable>")
		}), c)

		if recovered != nil {
			t.Fatalf("recovered value = %v; want nil", recovered)
		}
		requireInternalServerError(t, err)
		if len(sender.messages) != 1 {
			t.Fatalf("Telegram messages = %d; want 1", len(sender.messages))
		}
		message, ok := sender.messages[0].(tgbotapi.MessageConfig)
		if !ok {
			t.Fatalf("Telegram message type = %T; want tgbotapi.MessageConfig", sender.messages[0])
		}
		if message.ChatID != -1001234567890 {
			t.Errorf("chat ID = %d; want %d", message.ChatID, int64(-1001234567890))
		}
		if message.ParseMode != tgbotapi.ModeHTML {
			t.Errorf("parse mode = %q; want %q", message.ParseMode, tgbotapi.ModeHTML)
		}
		if !message.DisableWebPagePreview {
			t.Error("web page previews are enabled; want disabled")
		}
		for _, fragment := range []string{
			"AI Translate API caught a panic",
			"<b>Incident:</b>",
			"POST /api/v1/translate",
			"request-123",
			"&lt;database &amp; provider unavailable&gt;",
			"Full stack trace is preserved in the application logs.",
		} {
			if !strings.Contains(message.Text, fragment) {
				t.Errorf("alert does not contain %q\nalert: %s", fragment, message.Text)
			}
		}
		if strings.Contains(message.Text, "private-query") {
			t.Errorf("alert contains request query: %s", message.Text)
		}
		if observed.Len() != 1 || observed.All()[0].Message != "panic recovered" {
			t.Fatalf("panic logs = %v; want one panic recovered log", observed.All())
		}
		fields := observed.All()[0].ContextMap()
		stack, ok := fields["stack_trace"].(string)
		if !ok || !strings.Contains(stack, "TestMiddleware") {
			t.Errorf("panic log does not contain the full stack trace")
		}
	})

	t.Run("Telegram error does not replace recovered response", func(t *testing.T) {
		log, observed := observedLogger()
		sender := &recordingTelegramSender{sendErr: errors.New("Telegram unavailable")}
		c := newTestContext(http.MethodGet, "/api/v1/languages")
		c.SetPath("/api/v1/languages")
		SetTelegramAlertTarget(c, sender, 42)

		err, recovered := invoke(Middleware(log)(func(*echo.Context) error {
			panic("unexpected state")
		}), c)

		if recovered != nil {
			t.Fatalf("recovered value = %v; want nil", recovered)
		}
		requireInternalServerError(t, err)
		if observed.FilterMessage("panic alert delivery failed").Len() != 1 {
			t.Fatalf("delivery failure logs = %d; want 1", observed.FilterMessage("panic alert delivery failed").Len())
		}
	})

	t.Run("Telegram panic does not escape recovery", func(t *testing.T) {
		log, observed := observedLogger()
		sender := &recordingTelegramSender{panicValue: "sender failure"}
		c := newTestContext(http.MethodGet, "/api/v1/languages")
		c.SetPath("/api/v1/languages")
		SetTelegramAlertTarget(c, sender, 42)

		err, recovered := invoke(Middleware(log)(func(*echo.Context) error {
			panic("unexpected state")
		}), c)

		if recovered != nil {
			t.Fatalf("recovered value = %v; want nil", recovered)
		}
		requireInternalServerError(t, err)
		if observed.FilterMessage("panic alert delivery failed").Len() != 1 {
			t.Fatalf("delivery failure logs = %d; want 1", observed.FilterMessage("panic alert delivery failed").Len())
		}
	})

	t.Run("aborted request preserves net/http behavior", func(t *testing.T) {
		log, observed := observedLogger()
		sender := &recordingTelegramSender{}
		c := newTestContext(http.MethodGet, "/aborted")
		SetTelegramAlertTarget(c, sender, 42)

		err, recovered := invoke(Middleware(log)(func(*echo.Context) error {
			panic(http.ErrAbortHandler)
		}), c)

		if err != nil {
			t.Fatalf("error = %v; want nil", err)
		}
		if recovered != http.ErrAbortHandler {
			t.Fatalf("recovered value = %v; want http.ErrAbortHandler", recovered)
		}
		if len(sender.messages) != 0 {
			t.Fatalf("Telegram messages = %d; want 0", len(sender.messages))
		}
		if observed.Len() != 0 {
			t.Fatalf("error logs = %d; want 0", observed.Len())
		}
	})
}

// TestSetTelegramAlertTarget verifies the request-scoped alert target requires
// both a sender and a non-zero chat identifier.
func TestSetTelegramAlertTarget(t *testing.T) {
	c := newTestContext(http.MethodGet, "/health")
	if panicValue := panicFrom(func() { SetTelegramAlertTarget(c, &recordingTelegramSender{}, -42) }); panicValue != nil {
		t.Fatalf("SetTelegramAlertTarget(valid) panic = %v", panicValue)
	}
	for _, test := range []struct {
		name string
		call func()
	}{
		{name: "nil sender", call: func() { SetTelegramAlertTarget(c, nil, 42) }},
		{name: "zero chat ID", call: func() { SetTelegramAlertTarget(c, &recordingTelegramSender{}, 0) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if panicFrom(test.call) == nil {
				t.Fatal("SetTelegramAlertTarget() did not panic")
			}
		})
	}
}

func panicFrom(fn func()) (recovered any) {
	defer func() { recovered = recover() }()
	fn()
	return nil
}

func newTestContext(method string, target string) *echo.Context {
	e := echo.New()
	return e.NewContext(httptest.NewRequest(method, target, nil), httptest.NewRecorder())
}

func observedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, observed := observer.New(zap.ErrorLevel)
	return zap.New(core), observed
}

func invoke(handler echo.HandlerFunc, c *echo.Context) (err error, recovered any) {
	defer func() {
		recovered = recover()
	}()
	err = handler(c)
	return err, nil
}

func requireInternalServerError(t *testing.T, err error) {
	t.Helper()

	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %v; want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusInternalServerError {
		t.Errorf("status code = %d; want %d", httpErr.Code, http.StatusInternalServerError)
	}
	if httpErr.Message != pkg.SmthWW {
		t.Errorf("message = %q; want %q", httpErr.Message, pkg.SmthWW)
	}
}
