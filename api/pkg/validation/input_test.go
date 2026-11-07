package validation

import (
	"errors"
	"strings"
	"testing"
)

// TestValidateTranslation covers normalization, exact byte boundaries, invalid
// text, and invalid validator configuration.
func TestValidateTranslation(t *testing.T) {
	catalog := testCatalog(t)
	input := TranslationInput{Provider: " CHATGPT ", SourceLang: "english", TargetLang: "spanish", Text: "éé"}
	got, err := ValidateTranslation(catalog, input, 4)
	if err != nil {
		t.Fatalf("ValidateTranslation() error = %v", err)
	}
	wantLanguages := LanguageSelection{Provider: ProviderChatGPT, Source: "English", Target: "Spanish"}
	if got.Languages != wantLanguages || got.Text != input.Text {
		t.Fatalf("ValidateTranslation() = %#v; want languages %#v and text %q", got, wantLanguages, input.Text)
	}

	tests := []struct {
		name    string
		catalog *Catalog
		input   TranslationInput
		limit   int
		wantErr error
	}{
		{name: "nil catalog", input: input, limit: 4},
		{name: "invalid limit", catalog: catalog, input: input},
		{name: "empty text", catalog: catalog, input: TranslationInput{Provider: "chatgpt", TargetLang: "Spanish"}, limit: 4, wantErr: ErrInvalidText},
		{name: "text over byte limit", catalog: catalog, input: TranslationInput{Provider: "chatgpt", TargetLang: "Spanish", Text: "ééx"}, limit: 4, wantErr: ErrInvalidText},
		{name: "invalid languages", catalog: catalog, input: TranslationInput{Provider: "unknown", Text: "ok"}, limit: 4, wantErr: ErrUnsupportedProvider},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateTranslation(test.catalog, test.input, test.limit)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("ValidateTranslation() error = %v; want %v", err, test.wantErr)
				}
			} else if err == nil {
				t.Fatal("ValidateTranslation() error = nil; want validation error")
			}
		})
	}
}

// TestValidateInterpretationText covers the documented one-to-21-word range
// while preserving the caller's original text.
func TestValidateInterpretationText(t *testing.T) {
	valid := "  one\ttwo  "
	if got, err := ValidateInterpretationText(valid); err != nil || got != valid {
		t.Fatalf("ValidateInterpretationText() = (%q, %v); want original text, nil", got, err)
	}
	if _, err := ValidateInterpretationText(strings.Repeat("word ", MaxInterpretationWords)); err != nil {
		t.Fatalf("ValidateInterpretationText(21 words) error = %v", err)
	}
	for _, text := range []string{" \t\n", strings.Repeat("word ", MaxInterpretationWords+1)} {
		if _, err := ValidateInterpretationText(text); !errors.Is(err, ErrInvalidText) {
			t.Fatalf("ValidateInterpretationText(%q) error = %v; want %v", text, err, ErrInvalidText)
		}
	}
}

// TestValidateApplicationFeedback covers both accepted ratings and the
// optional text's byte-length boundary.
func TestValidateApplicationFeedback(t *testing.T) {
	for _, rating := range []int{-1, 1} {
		feedback, err := ValidateApplicationFeedback(rating, strings.Repeat("é", MaxFeedbackTextBytes/2))
		if err != nil || feedback.Rating != rating {
			t.Fatalf("ValidateApplicationFeedback(%d) = (%#v, %v)", rating, feedback, err)
		}
	}
	if _, err := ValidateApplicationFeedback(0, ""); !errors.Is(err, ErrInvalidRating) {
		t.Fatalf("invalid rating error = %v; want %v", err, ErrInvalidRating)
	}
	if _, err := ValidateApplicationFeedback(1, strings.Repeat("é", MaxFeedbackTextBytes/2+1)); !errors.Is(err, ErrInvalidText) {
		t.Fatalf("oversized feedback error = %v; want %v", err, ErrInvalidText)
	}
}

// TestValidateEmail covers whitespace normalization, plain-address validation,
// display-name rejection, and the documented length range.
func TestValidateEmail(t *testing.T) {
	if got, err := ValidateEmail("  reader@example.com  "); err != nil || got != "reader@example.com" {
		t.Fatalf("ValidateEmail() = (%q, %v); want normalized address", got, err)
	}
	invalid := []string{"", "a@", "Reader <reader@example.com>", strings.Repeat("a", 321)}
	for _, email := range invalid {
		if _, err := ValidateEmail(email); !errors.Is(err, ErrInvalidEmail) {
			t.Fatalf("ValidateEmail(%q) error = %v; want %v", email, err, ErrInvalidEmail)
		}
	}
}

// TestValidateTranslationFeedback covers normalized metadata and assessment,
// both text fields, and invalid validator configuration.
func TestValidateTranslationFeedback(t *testing.T) {
	catalog := testCatalog(t)
	input := TranslationFeedbackInput{
		TranslationID:   "8bdf29a7-54d2-48ee-bd2b-c0ae79af6831",
		Revision:        1,
		Provider:        " CHATGPT ",
		SourceLang:      "english",
		TargetLang:      "spanish",
		SourceText:      "hello",
		TranslationText: "hola",
		Assessment:      " Like ",
	}
	got, err := ValidateTranslationFeedback(catalog, input, 5)
	if err != nil {
		t.Fatalf("ValidateTranslationFeedback() error = %v", err)
	}
	if got.TranslationID.String() != input.TranslationID || got.Revision != 1 || got.Assessment != "like" || got.SourceText != "hello" || got.TranslationText != "hola" || got.Languages.Provider != ProviderChatGPT {
		t.Fatalf("ValidateTranslationFeedback() = %#v; want normalized feedback", got)
	}

	tests := []struct {
		name    string
		catalog *Catalog
		input   TranslationFeedbackInput
		limit   int
		wantErr error
	}{
		{name: "nil catalog", input: input, limit: 5},
		{name: "invalid limit", catalog: catalog, input: input},
		{name: "invalid translation ID", catalog: catalog, input: withFeedback(input, func(value *TranslationFeedbackInput) { value.TranslationID = "not-a-uuid" }), limit: 5, wantErr: ErrInvalidTranslationID},
		{name: "nil translation ID", catalog: catalog, input: withFeedback(input, func(value *TranslationFeedbackInput) { value.TranslationID = "00000000-0000-0000-0000-000000000000" }), limit: 5, wantErr: ErrInvalidTranslationID},
		{name: "invalid revision", catalog: catalog, input: withFeedback(input, func(value *TranslationFeedbackInput) { value.Revision = 0 }), limit: 5, wantErr: ErrInvalidRevision},
		{name: "invalid provider", catalog: catalog, input: withFeedback(input, func(value *TranslationFeedbackInput) { value.Provider = "unknown" }), limit: 5, wantErr: ErrUnsupportedProvider},
		{name: "missing source text", catalog: catalog, input: withFeedback(input, func(value *TranslationFeedbackInput) { value.SourceText = "" }), limit: 5, wantErr: ErrInvalidText},
		{name: "oversized translation text", catalog: catalog, input: withFeedback(input, func(value *TranslationFeedbackInput) { value.TranslationText = "ééé" }), limit: 5, wantErr: ErrInvalidText},
		{name: "invalid assessment", catalog: catalog, input: withFeedback(input, func(value *TranslationFeedbackInput) { value.Assessment = "neutral" }), limit: 5, wantErr: ErrInvalidAssessment},
		{name: "dislike accepted", catalog: catalog, input: withFeedback(input, func(value *TranslationFeedbackInput) { value.Assessment = "DISLIKE" }), limit: 5},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateTranslationFeedback(test.catalog, test.input, test.limit)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("ValidateTranslationFeedback() error = %v; want %v", err, test.wantErr)
				}
			} else if test.catalog == nil || test.limit <= 0 {
				if err == nil {
					t.Fatal("ValidateTranslationFeedback() error = nil; want configuration error")
				}
			} else if err != nil {
				t.Fatalf("ValidateTranslationFeedback() error = %v", err)
			}
		})
	}
}

func withFeedback(input TranslationFeedbackInput, change func(*TranslationFeedbackInput)) TranslationFeedbackInput {
	change(&input)
	return input
}
