// Package providerfailure converts terminal upstream failures into panics that
// the HTTP recovery middleware can report to operators.
package providerfailure

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/openai/openai-go/v3"
)

var openAIQuotaCodes = map[string]struct{}{
	"credit_balance_exhausted":          {},
	"organization_spend_limit_exceeded": {},
	"organization_usage_limit_exceeded": {},
	"project_spend_limit_exceeded":      {},
	"usage_limit_exceeded":              {},
}

// PanicIfOpenAIQuotaExceeded turns a recognized billing or usage-limit error
// into an operator-visible incident. Other errors are left to the caller's
// retry policy.
func PanicIfOpenAIQuotaExceeded(operation string, err error) {
	operation = validation.MustNotBlank("operation", operation)
	if err == nil {
		return
	}
	apiErr, ok := asOpenAIError(err)
	if !ok || !isOpenAIQuotaError(apiErr) {
		return
	}

	panic(openAIFailure(operation, apiErr, "quota exhausted"))
}

// PanicOpenAI reports a terminal OpenAI failure without placing provider
// response text, request content, or credentials in logs and Telegram alerts.
func PanicOpenAI(operation string, err error) {
	operation = validation.MustNotBlank("operation", operation)
	validation.MustNotNil("OpenAI error", err)
	if apiErr, ok := asOpenAIError(err); ok {
		reason := "request failed"
		if isOpenAIQuotaError(apiErr) {
			reason = "quota exhausted"
		}
		panic(openAIFailure(operation, apiErr, reason))
	}

	panic(fmt.Errorf("%s: OpenAI request failed after retries", operation))
}

// PanicOpenAIResponse reports a terminal streaming state using only the state
// and code supplied by the caller; it intentionally accepts no free-form
// provider message.
func PanicOpenAIResponse(operation string, state string, code string) {
	operation = validation.MustNotBlank("operation", operation)
	state = validation.MustNotBlank("OpenAI response state", state)
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
