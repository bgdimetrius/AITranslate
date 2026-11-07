package interpretate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"go.uber.org/zap"
)

type interpretationServiceDBStub struct{}

type interpretationRoundTripFunc func(*http.Request) (*http.Response, error)

func (f interpretationRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func (interpretationServiceDBStub) ValidateUserId(context.Context, int64) (bool, error) {
	return true, nil
}

func (interpretationServiceDBStub) RecordDailyUsage(context.Context, queries.RecordDailyUsageParams) error {
	return nil
}

// Both OpenAI operations must reach panic recovery as soon as the provider
// reports a hard quota limit.
func TestInterpretationServicePanicsOnOpenAIQuotaExhaustion(t *testing.T) {
	httpClient := &http.Client{Transport: interpretationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Status:     "429 Too Many Requests",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"monthly limit reached","type":"insufficient_quota","code":"organization_usage_limit_exceeded"}}`)),
			Request:    request,
		}, nil
	})}
	client := openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithHTTPClient(httpClient),
		option.WithMaxRetries(0),
	)
	service := &interpservice{
		model:         shared.ChatModel("test-model"),
		client:        client,
		db:            interpretationServiceDBStub{},
		prompt:        "interpret",
		analyzeprompt: "analyze",
	}

	tests := []struct {
		name string
		call func()
	}{
		{
			name: "interpretation",
			call: func() {
				_, _, _ = service.Interpretate(context.Background(), zap.NewNop(), &interpReq{
					TargetLang: "Spanish",
					UserId:     42,
					Text:       "hello",
				})
			},
		},
		{
			name: "text analysis",
			call: func() {
				_, _, _ = service.AnalyzeText(context.Background(), zap.NewNop(), "hello")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			panicValue := captureInterpretationPanic(test.call)
			message := fmt.Sprint(panicValue)
			if !strings.Contains(message, "OpenAI quota exhausted") || !strings.Contains(message, "organization_usage_limit_exceeded") {
				t.Fatalf("panic = %q; want sanitized OpenAI quota detail", message)
			}
		})
	}
}

func captureInterpretationPanic(f func()) (recovered any) {
	defer func() {
		recovered = recover()
	}()
	f()
	return nil
}
