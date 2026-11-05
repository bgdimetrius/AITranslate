package providerfailure

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
)

// TestPanicIfOpenAIQuotaExceeded verifies that permanent quota failures alert
// immediately while temporary rate limits remain eligible for SDK retries.
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

// TestPanicOpenAISanitizesProviderError ensures Telegram alerts identify the
// actionable provider code without copying arbitrary upstream response text.
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
