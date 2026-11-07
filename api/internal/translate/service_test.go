package translate

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cluttrdev/deepl-go/deepl"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"go.uber.org/zap"
)

type deepLClientStub struct {
	usage            *deepl.Usage
	usageErr         error
	translations     []deepl.Translation
	translateErr     error
	usageCalls       int
	translationCalls int
}

type translationRoundTripFunc func(*http.Request) (*http.Response, error)

func (f translationRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func (s *deepLClientStub) GetUsage() (*deepl.Usage, error) {
	s.usageCalls++
	return s.usage, s.usageErr
}

func (s *deepLClientStub) TranslateText(_ []string, _ string, _ ...deepl.TranslateOption) ([]deepl.Translation, error) {
	s.translationCalls++
	return s.translations, s.translateErr
}

// One depleted account must not deny service while another configured account
// can fit the complete request.
func TestDeepLTranslatorSkipsExhaustedAccounts(t *testing.T) {
	exhausted := &deepLClientStub{usage: &deepl.Usage{CharacterCount: 99, CharacterLimit: 100}}
	available := &deepLClientStub{
		usage:        &deepl.Usage{CharacterCount: 98, CharacterLimit: 100},
		translations: []deepl.Translation{{DetectedSourceLanguage: "EN", Text: "hola"}},
	}
	translator := &deeplTranslator{translators: []deepLClient{exhausted, available}}
	var output bytes.Buffer

	_, err := translator.Do(context.Background(), zap.NewNop(), &output, validTranslationRequest("éé"))

	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if output.String() != `{"source_lang":"EN","text":"hola"}` {
		t.Fatalf("output = %q; want translated JSON", output.String())
	}
	if exhausted.translationCalls != 0 {
		t.Fatalf("exhausted translator calls = %d; want 0", exhausted.translationCalls)
	}
	if available.translationCalls != 1 {
		t.Fatalf("available translator calls = %d; want 1", available.translationCalls)
	}
}

// Quota exhaustion is an operator incident only when every configured account
// is unable to fit the request.
func TestDeepLTranslatorPanicsOnlyWhenEveryAccountIsExhausted(t *testing.T) {
	first := &deepLClientStub{usage: &deepl.Usage{CharacterCount: 100, CharacterLimit: 100}}
	second := &deepLClientStub{usage: &deepl.Usage{CharacterCount: 199, CharacterLimit: 200}}
	translator := &deeplTranslator{translators: []deepLClient{first, second}}

	panicValue := captureTranslationPanic(func() {
		_, _ = translator.Do(context.Background(), zap.NewNop(), &bytes.Buffer{}, validTranslationRequest("two"))
	})

	message := fmt.Sprint(panicValue)
	if !strings.Contains(message, "all 2 configured translators") {
		t.Fatalf("panic = %q; want all-translators quota detail", message)
	}
	if first.translationCalls != 0 || second.translationCalls != 0 {
		t.Fatalf("translation calls = (%d, %d); want (0, 0)", first.translationCalls, second.translationCalls)
	}
}

// Provider-side quota rejection is authoritative because usage may change
// between the preflight check and the translation request.
func TestDeepLTranslatorFallsThroughAfterRacingQuotaChange(t *testing.T) {
	first := &deepLClientStub{
		usage:        &deepl.Usage{CharacterCount: 0, CharacterLimit: 100},
		translateErr: fmt.Errorf("456 - Quota exceeded. The character limit has been reached."),
	}
	second := &deepLClientStub{
		usage:        &deepl.Usage{CharacterCount: 0, CharacterLimit: 100},
		translations: []deepl.Translation{{DetectedSourceLanguage: "EN", Text: "hola"}},
	}
	translator := &deeplTranslator{translators: []deepLClient{first, second}}

	panicValue := captureTranslationPanic(func() {
		_, err := translator.Do(context.Background(), zap.NewNop(), &bytes.Buffer{}, validTranslationRequest("hello"))
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
	})

	if panicValue != nil {
		t.Fatalf("Do() panic = %v; want nil", panicValue)
	}
	if first.translationCalls != 1 || second.translationCalls != 1 {
		t.Fatalf("translation calls = (%d, %d); want (1, 1)", first.translationCalls, second.translationCalls)
	}
}

// Hard quota failures must reach panic recovery so operators receive an alert
// instead of an unexplained 500.
func TestGPTTranslatorPanicsOnQuotaExhaustion(t *testing.T) {
	httpClient := &http.Client{Transport: translationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Status:     "429 Too Many Requests",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"no credits","type":"insufficient_quota","code":"credit_balance_exhausted"}}`)),
			Request:    request,
		}, nil
	})}
	client := openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithHTTPClient(httpClient),
		option.WithMaxRetries(0),
	)
	translator := &gptTranslator{client: client, model: shared.ChatModel("test-model"), prompt: "translate"}

	panicValue := captureTranslationPanic(func() {
		_, _ = translator.Do(context.Background(), zap.NewNop(), &bytes.Buffer{}, validTranslationRequest("hello"))
	})

	message := fmt.Sprint(panicValue)
	if !strings.Contains(message, "OpenAI quota exhausted") || !strings.Contains(message, "credit_balance_exhausted") {
		t.Fatalf("panic = %q; want sanitized OpenAI quota detail", message)
	}
}

// An HTTP 200 stream can still terminate in a provider failure; that terminal
// event must never be mistaken for a completed translation.
func TestGPTTranslatorPanicsOnFailedStream(t *testing.T) {
	httpClient := &http.Client{Transport: translationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := "event: response.failed\n" +
			`data: {"type":"response.failed","sequence_number":1,"response":{"error":{"code":"server_error","message":"sensitive upstream detail"}}}` + "\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
	client := openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithHTTPClient(httpClient),
		option.WithMaxRetries(0),
	)
	translator := &gptTranslator{client: client, model: shared.ChatModel("test-model"), prompt: "translate"}

	panicValue := captureTranslationPanic(func() {
		_, _ = translator.Do(context.Background(), zap.NewNop(), &bytes.Buffer{}, validTranslationRequest("hello"))
	})

	message := fmt.Sprint(panicValue)
	if !strings.Contains(message, "response failed (code server_error)") {
		t.Fatalf("panic = %q; want stream failure code", message)
	}
	if strings.Contains(message, "sensitive upstream detail") {
		t.Fatalf("panic leaked provider response: %q", message)
	}
}

func validTranslationRequest(text string) *translationReq {
	return &translationReq{TargetLang: "ES", Text: text, allowedLength: 1_000}
}

func captureTranslationPanic(f func()) (recovered any) {
	defer func() {
		recovered = recover()
	}()
	f()
	return nil
}
