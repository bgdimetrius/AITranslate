package analytics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bgdimetrius/AITranslate/internal/languages"
	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

type analyticsServiceStub struct {
	feedbackErr         error
	leadErr             error
	translationErr      error
	userID              int
	feedback            *rating
	leadEmail           string
	translationFeedback *validation.TranslationFeedback
}

func (service *analyticsServiceStub) SaveApplicationFeedback(_ context.Context, _ *zap.Logger, userID int, feedback rating) error {
	service.userID = userID
	feedbackCopy := feedback
	service.feedback = &feedbackCopy
	return service.feedbackErr
}

func (service *analyticsServiceStub) SaveFeedbackLead(_ context.Context, _ *zap.Logger, userID int, email string) error {
	service.userID = userID
	service.leadEmail = email
	return service.leadErr
}

func (service *analyticsServiceStub) SaveTranslationFeedback(_ context.Context, _ *zap.Logger, userID int, feedback validation.TranslationFeedback) error {
	service.userID = userID
	feedbackCopy := feedback
	service.translationFeedback = &feedbackCopy
	return service.translationErr
}

func newAnalyticsContext(method, target, body string) (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c := e.NewContext(request, recorder)
	c.Set("logger", zap.NewNop())
	c.Set("user_id", 42)
	return c, recorder
}

// TestAnalyticsHandlerApplicationFeedback covers accepted ratings, the byte
// boundary, malformed input, and persistence failure.
func TestAnalyticsHandlerApplicationFeedback(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		serviceErr   error
		wantCode     int
		wantFeedback *rating
	}{
		{
			name:         "positive rating at 128-byte boundary",
			body:         `{"rating":1,"text":"` + strings.Repeat("é", 64) + `"}`,
			wantCode:     http.StatusNoContent,
			wantFeedback: &rating{Rating: 1, Text: strings.Repeat("é", 64)},
		},
		{
			name:         "negative rating",
			body:         `{"rating":-1}`,
			wantCode:     http.StatusNoContent,
			wantFeedback: &rating{Rating: -1},
		},
		{name: "malformed JSON", body: `{"rating":`, wantCode: http.StatusBadRequest},
		{name: "unsupported rating", body: `{"rating":0}`, wantCode: http.StatusBadRequest},
		{name: "feedback over 128 bytes", body: `{"rating":1,"text":"` + strings.Repeat("é", 65) + `"}`, wantCode: http.StatusBadRequest},
		{
			name:         "persistence failure",
			body:         `{"rating":1,"text":"useful"}`,
			serviceErr:   errors.New("database unavailable"),
			wantCode:     http.StatusInternalServerError,
			wantFeedback: &rating{Rating: 1, Text: "useful"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &analyticsServiceStub{feedbackErr: test.serviceErr}
			handler := NewAnalyticsHandler(service)
			c, recorder := newAnalyticsContext(http.MethodPut, "/analytics/feedback", test.body)

			err := handler.ApplicationFeedback(c)
			if test.wantCode == http.StatusNoContent {
				if err != nil {
					t.Fatalf("ApplicationFeedback() error = %v", err)
				}
				if recorder.Code != test.wantCode {
					t.Fatalf("status = %d; want %d", recorder.Code, test.wantCode)
				}
			} else {
				wantMessage := pkg.InvalidInput
				if test.wantCode == http.StatusInternalServerError {
					wantMessage = pkg.SmthWW
				}
				requireAnalyticsHTTPError(t, err, test.wantCode, wantMessage)
			}

			wantUserID := 0
			if test.wantFeedback != nil {
				wantUserID = 42
			}
			if service.userID != wantUserID {
				t.Fatalf("service user ID = %d; want %d", service.userID, wantUserID)
			}
			if !equalRating(service.feedback, test.wantFeedback) {
				t.Fatalf("service feedback = %#v; want %#v", service.feedback, test.wantFeedback)
			}
		})
	}
}

// TestAnalyticsHandlerRegisterFeedbackLead covers whitespace trimming, plain
// address validation, malformed input, and persistence failure.
func TestAnalyticsHandlerRegisterFeedbackLead(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		serviceErr error
		wantCode   int
		wantEmail  string
	}{
		{name: "trimmed plain email", body: `{"email":"  reader@example.com  "}`, wantCode: http.StatusNoContent, wantEmail: "reader@example.com"},
		{name: "malformed JSON", body: `{"email":`, wantCode: http.StatusBadRequest},
		{name: "invalid email", body: `{"email":"not-an-email"}`, wantCode: http.StatusBadRequest},
		{name: "display-name address", body: `{"email":"Reader <reader@example.com>"}`, wantCode: http.StatusBadRequest},
		{
			name:       "persistence failure",
			body:       `{"email":"reader@example.com"}`,
			serviceErr: errors.New("database unavailable"),
			wantCode:   http.StatusInternalServerError,
			wantEmail:  "reader@example.com",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &analyticsServiceStub{leadErr: test.serviceErr}
			handler := NewAnalyticsHandler(service)
			c, recorder := newAnalyticsContext(http.MethodPut, "/analytics/feedback/order", test.body)

			err := handler.RegisterFeedbackLead(c)
			if test.wantCode == http.StatusNoContent {
				if err != nil {
					t.Fatalf("RegisterFeedbackLead() error = %v", err)
				}
				if recorder.Code != test.wantCode {
					t.Fatalf("status = %d; want %d", recorder.Code, test.wantCode)
				}
			} else {
				wantMessage := pkg.InvalidInput
				if test.wantCode == http.StatusInternalServerError {
					wantMessage = pkg.SmthWW
				}
				requireAnalyticsHTTPError(t, err, test.wantCode, wantMessage)
			}

			wantUserID := 0
			if test.wantEmail != "" {
				wantUserID = 42
			}
			if service.userID != wantUserID {
				t.Fatalf("service user ID = %d; want %d", service.userID, wantUserID)
			}
			if service.leadEmail != test.wantEmail {
				t.Fatalf("service email = %q; want %q", service.leadEmail, test.wantEmail)
			}
		})
	}
}

// TestAnalyticsHandlerTranslationFeedback covers validation, normalized service
// input, successful persistence, and persistence failure.
func TestAnalyticsHandlerTranslationFeedback(t *testing.T) {
	validBody := `{"translation_id":"8bdf29a7-54d2-48ee-bd2b-c0ae79af6831","revision":1,"translator":"chatgpt","source_lang":"english","target_lang":"spanish","source_text":"hello","translation_text":"hola","assessment":"LIKE"}`
	validFeedback := validation.TranslationFeedback{
		TranslationID:   uuid.MustParse("8bdf29a7-54d2-48ee-bd2b-c0ae79af6831"),
		Revision:        1,
		Languages:       validation.LanguageSelection{Provider: validation.ProviderChatGPT, Source: "English", Target: "Spanish"},
		SourceText:      "hello",
		TranslationText: "hola",
		Assessment:      "like",
	}
	tests := []struct {
		name         string
		body         string
		userID       any
		legacy       bool
		serviceErr   error
		wantCode     int
		wantMessage  string
		wantFeedback *validation.TranslationFeedback
	}{
		{name: "valid feedback is persisted", body: validBody, userID: 42, wantCode: http.StatusNoContent, wantFeedback: &validFeedback},
		{name: "persistence failure", body: validBody, userID: 42, serviceErr: errors.New("database unavailable"), wantCode: http.StatusInternalServerError, wantMessage: pkg.SmthWW, wantFeedback: &validFeedback},
		{name: "malformed JSON", body: `{`, userID: 42, wantCode: http.StatusBadRequest, wantMessage: pkg.InvalidInput},
		{name: "invalid translation ID", body: strings.Replace(validBody, `"8bdf29a7-54d2-48ee-bd2b-c0ae79af6831"`, `"invalid"`, 1), userID: 42, wantCode: http.StatusBadRequest, wantMessage: pkg.InvalidInput},
		{name: "invalid revision", body: strings.Replace(validBody, `"revision":1`, `"revision":0`, 1), userID: 42, wantCode: http.StatusBadRequest, wantMessage: pkg.InvalidInput},
		{name: "unsupported provider", body: strings.Replace(validBody, `"chatgpt"`, `"unknown"`, 1), userID: 42, wantCode: http.StatusBadRequest, wantMessage: pkg.InvalidInput},
		{name: "unsupported source", body: strings.Replace(validBody, `"english"`, `"german"`, 1), userID: 42, wantCode: http.StatusBadRequest, wantMessage: pkg.InvalidInput},
		{name: "invalid assessment", body: strings.Replace(validBody, `"LIKE"`, `"neutral"`, 1), userID: 42, wantCode: http.StatusBadRequest, wantMessage: pkg.InvalidInput},
		{name: "text over configured byte limit", body: strings.Replace(validBody, `"hello"`, `"ééé"`, 1), userID: 42, wantCode: http.StatusBadRequest, wantMessage: pkg.InvalidInput},
		{name: "missing authenticated installation", body: validBody, wantCode: http.StatusInternalServerError, wantMessage: pkg.SmthWW},
		{name: "handler without translation validation reports configuration failure", body: validBody, userID: 42, legacy: true, wantCode: http.StatusInternalServerError, wantMessage: pkg.SmthWW},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &analyticsServiceStub{translationErr: test.serviceErr}
			var handler AnalyticsHandler
			if test.legacy {
				handler = NewAnalyticsHandler(service)
			} else {
				handler = NewAnalyticsHandlerWithValidation(service, analyticsCatalog(t), 5)
			}
			c, recorder := newAnalyticsContext(http.MethodPut, "/analytics/feedback/translation", test.body)
			c.Set(pkg.UserId, test.userID)

			err := handler.TranslationFeedback(c)
			if test.wantCode == http.StatusNoContent {
				if err != nil {
					t.Fatalf("TranslationFeedback() error = %v", err)
				}
				if recorder.Code != test.wantCode {
					t.Fatalf("status = %d; want %d", recorder.Code, test.wantCode)
				}
			} else {
				requireAnalyticsHTTPError(t, err, test.wantCode, test.wantMessage)
			}
			if service.feedback != nil || service.leadEmail != "" {
				t.Fatal("translation feedback reached an unrelated persistence method")
			}
			if !equalTranslationFeedback(service.translationFeedback, test.wantFeedback) {
				t.Fatalf("service translation feedback = %#v; want %#v", service.translationFeedback, test.wantFeedback)
			}
		})
	}
}

// TestAnalyticsConstructors verifies the documented dependency and limit
// preconditions for handler and service construction.
func TestAnalyticsConstructors(t *testing.T) {
	service := new(analyticsServiceStub)
	tests := []struct {
		name string
		call func()
	}{
		{name: "nil handler service", call: func() { NewAnalyticsHandler(nil) }},
		{name: "nil validated handler service", call: func() { NewAnalyticsHandlerWithValidation(nil, analyticsCatalog(t), 5) }},
		{name: "nil language catalog", call: func() { NewAnalyticsHandlerWithValidation(service, nil, 5) }},
		{name: "non-positive text limit", call: func() { NewAnalyticsHandlerWithValidation(service, analyticsCatalog(t), 0) }},
		{name: "nil service database", call: func() { NewAnalyticsService(nil) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !analyticsPanics(test.call) {
				t.Fatal("constructor did not panic")
			}
		})
	}
}

func analyticsCatalog(t *testing.T) *languages.Catalog {
	t.Helper()
	catalog, err := languages.NewCatalog(map[languages.Provider]languages.Capabilities{
		languages.ProviderChatGPT: {Source: []string{"English"}, Target: []string{"Spanish"}, AutoDetect: true},
	})
	if err != nil {
		t.Fatalf("create analytics catalog: %v", err)
	}
	return catalog
}

func analyticsPanics(fn func()) (didPanic bool) {
	defer func() { didPanic = recover() != nil }()
	fn()
	return false
}

func equalRating(got, want *rating) bool {
	if got == nil || want == nil {
		return got == want
	}
	return *got == *want
}

func equalTranslationFeedback(got, want *validation.TranslationFeedback) bool {
	if got == nil || want == nil {
		return got == want
	}
	return *got == *want
}

func requireAnalyticsHTTPError(t *testing.T, err error, code int, message string) {
	t.Helper()
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %v; want *echo.HTTPError", err)
	}
	if httpErr.Code != code {
		t.Fatalf("HTTP error code = %d; want %d", httpErr.Code, code)
	}
	if httpErr.Message != message {
		t.Fatalf("HTTP error message = %v; want %q", httpErr.Message, message)
	}
}
