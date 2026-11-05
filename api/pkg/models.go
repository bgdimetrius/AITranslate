// Package pkg defines response messages and accounting values shared across API
// features.
package pkg

// Stable client-facing error messages keep internal failures out of responses.
const (
	InvalidInput    = "invalid input"
	SmthWW          = "something went wrong"
	RequestTimedOut = "request timed out"
)

// ErrorResponse is the JSON envelope emitted by Echo's global error handler.
type ErrorResponse struct {
	Message string `json:"message" example:"invalid input"`
}

// Usage contains additive counters for one provider operation. Zero values mean
// the corresponding activity was not observed or reported by the provider.
type Usage struct {
	TranslationRequests    int32
	InterpretationRequests int32
	FailedRequests         int32
	// InputCharacters counts Unicode code points rather than UTF-8 bytes.
	InputCharacters int64
	// Token counts are provider-reported and remain zero for providers that do
	// not expose them.
	InputTokens  int64
	OutputTokens int64
}
