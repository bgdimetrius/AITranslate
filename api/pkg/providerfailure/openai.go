// Package providerfailure converts terminal upstream failures into panics that
// the HTTP recovery middleware can report to operators.
package providerfailure

import (
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
)

var openAIQuotaCodes = map[string]struct{}{
	"credit_balance_exhausted":          {},
	"organization_spend_limit_exceeded": {},
	"organization_usage_limit_exceeded": {},
	"project_spend_limit_exceeded":      {},
	"usage_limit_exceeded":              {},
}

// PanicIfOpenAIQuotaExceeded turns a non-retriable billing or usage limit into
// an operator-visible incident. OpenAI does not expose a supported preflight
// balance check to ordinary project API keys, so the failed request is the
// authoritative quota check.
func PanicIfOpenAIQuotaExceeded(operation string, err error) {
	apiErr, ok := asOpenAIError(err)
	if !ok || !isOpenAIQuotaError(apiErr) {
		return
	}

	panic(openAIFailure(operation, apiErr, "quota exhausted"))
}

// PanicOpenAI reports a terminal OpenAI failure without placing provider
// response text, request content, or credentials in logs and Telegram alerts.
func PanicOpenAI(operation string, err error) {
	if apiErr, ok := asOpenAIError(err); ok {
		reason := "request failed"
		if isOpenAIQuotaError(apiErr) {
			reason = "quota exhausted"
		}
		panic(openAIFailure(operation, apiErr, reason))
	}

	panic(fmt.Errorf("%s: OpenAI request failed after retries", operation))
}

// PanicOpenAIResponse reports a terminal streaming state using provider codes
// that are safe to expose, while intentionally excluding free-form messages.
func PanicOpenAIResponse(operation string, state string, code string) {
	code = strings.TrimSpace(code)
	if code == "" {
		code = "unknown"
	}
	panic(fmt.Errorf("%s: OpenAI %s (code %s)", operation, state, code))
}

func asOpenAIError(err error) (*openai.Error, bool) {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return nil, false
	}
	return apiErr, true
}

func isOpenAIQuotaError(err *openai.Error) bool {
	code := strings.ToLower(strings.TrimSpace(err.Code))
	if _, ok := openAIQuotaCodes[code]; ok {
		return true
	}

	return strings.EqualFold(strings.TrimSpace(err.Type), "insufficient_quota")
}

func openAIFailure(operation string, err *openai.Error, reason string) error {
	code := strings.TrimSpace(err.Code)
	if code == "" {
		code = strings.TrimSpace(err.Type)
	}
	if code == "" {
		code = "unknown"
	}

	return fmt.Errorf("%s: OpenAI %s (status %d, code %s)", operation, reason, err.StatusCode, code)
}
