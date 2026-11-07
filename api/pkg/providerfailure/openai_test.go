package providerfailure

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
)

// Permanent quota failures should alert immediately, while temporary rate
// limits remain eligible for the caller's retry policy.
func TestPanicIfOpenAIQuotaExceeded(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantPanic bool
	}{
		{
			name:      "credit balance exhausted",
			err:       &openai.Error{StatusCode: 429, Code: "credit_balance_exhausted", Message: "private provider detail"},
			wantPanic: true,
		},
		{
			name:      "legacy insufficient quota type",
			err:       fmt.Errorf("wrapped: %w", &openai.Error{StatusCode: 429, Type: "insufficient_quota"}),
			wantPanic: true,
		},
		{
			name: "temporary rate limit",
			err:  &openai.Error{StatusCode: 429, Code: "rate_limit_exceeded"},
		},
		{
			name: "network failure",
			err:  fmt.Errorf("connection reset"),
		},
		{
			name: "nil error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			panicValue := capturePanic(func() {
				PanicIfOpenAIQuotaExceeded("translation", test.err)
			})
			if test.wantPanic && panicValue == nil {
				t.Fatal("PanicIfOpenAIQuotaExceeded() did not panic")
			}
			if !test.wantPanic && panicValue != nil {
				t.Fatalf("PanicIfOpenAIQuotaExceeded() panic = %v; want nil", panicValue)
			}
		})
	}
}

// TestOpenAIFailurePreconditions verifies invalid operation names, missing
// terminal errors, and missing response states are rejected before reporting.
func TestOpenAIFailurePreconditions(t *testing.T) {
	tests := []struct {
		name string
		call func()
	}{
		{name: "blank quota operation", call: func() { PanicIfOpenAIQuotaExceeded(" ", nil) }},
		{name: "blank terminal operation", call: func() { PanicOpenAI(" ", errors.New("failure")) }},
		{name: "nil terminal error", call: func() { PanicOpenAI("translation", nil) }},
		{name: "blank response operation", call: func() { PanicOpenAIResponse(" ", "failed", "code") }},
		{name: "blank response state", call: func() { PanicOpenAIResponse("translation", " ", "code") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if capturePanic(test.call) == nil {
				t.Fatal("failure reporter did not panic")
			}
		})
	}
}

// TestPanicOpenAIResponse reports only the supplied state and a normalized
// provider code, defaulting an omitted code to unknown.
func TestPanicOpenAIResponse(t *testing.T) {
	panicValue := capturePanic(func() { PanicOpenAIResponse("translation", "response failed", "  ") })
	message := fmt.Sprint(panicValue)
	for _, fragment := range []string{"translation", "response failed", "code unknown"} {
		if !strings.Contains(message, fragment) {
			t.Errorf("panic = %q; want fragment %q", message, fragment)
		}
	}
}

// Alerts need the actionable provider code but must not copy arbitrary
// upstream response text.
func TestPanicOpenAISanitizesProviderError(t *testing.T) {
	panicValue := capturePanic(func() {
		PanicOpenAI("text analysis", &openai.Error{
			StatusCode: 429,
			Code:       "project_spend_limit_exceeded",
			Message:    "sensitive provider response",
		})
	})

	message := fmt.Sprint(panicValue)
	for _, fragment := range []string{"text analysis", "quota exhausted", "status 429", "project_spend_limit_exceeded"} {
		if !strings.Contains(message, fragment) {
			t.Errorf("panic = %q; want fragment %q", message, fragment)
		}
	}
	if strings.Contains(message, "sensitive provider response") {
		t.Errorf("panic leaked provider response: %q", message)
	}

	genericPanic := capturePanic(func() {
		PanicOpenAI("translation", errors.New("sensitive transport detail"))
	})
	if strings.Contains(fmt.Sprint(genericPanic), "sensitive transport detail") {
		t.Errorf("panic leaked transport error: %q", genericPanic)
	}
}

func capturePanic(f func()) (recovered any) {
	defer func() {
		recovered = recover()
	}()
	f()
	return nil
}
