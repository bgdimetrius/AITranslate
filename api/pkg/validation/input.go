package validation

import (
	"errors"
	"net/mail"
	"strings"

	"github.com/google/uuid"
)

const (
	MaxFeedbackTextBytes   = 128
	MaxInterpretationWords = 21
)

var (
	ErrInvalidText          = errors.New("invalid text")
	ErrInvalidRating        = errors.New("invalid rating")
	ErrInvalidEmail         = errors.New("invalid email")
	ErrInvalidAssessment    = errors.New("invalid assessment")
	ErrInvalidTranslationID = errors.New("invalid translation ID")
	ErrInvalidRevision      = errors.New("invalid revision")
)

// TranslationInput is raw translation data extracted by a transport adapter.
type TranslationInput struct {
	Provider   string
	SourceLang string
	TargetLang string
	Text       string
}

// Translation is normalized input safe to pass to the translation service.
type Translation struct {
	Languages LanguageSelection
	Text      string
}

// ValidateTranslation validates a complete translation request.
func ValidateTranslation(catalog *Catalog, input TranslationInput, maxTextBytes int) (Translation, error) {
	if catalog == nil {
		return Translation{}, errors.New("language catalog is nil")
	}
	if err := PositiveInt("max text bytes", maxTextBytes); err != nil {
		return Translation{}, err
	}

	languages, err := catalog.ValidateSelection(input.Provider, input.SourceLang, input.TargetLang)
	if err != nil {
		return Translation{}, err
	}
	if input.Text == "" || len(input.Text) > maxTextBytes {
		return Translation{}, ErrInvalidText
	}

	return Translation{Languages: languages, Text: input.Text}, nil
}

// ValidateInterpretationText enforces the shared short-text word limit.
func ValidateInterpretationText(text string) (string, error) {
	if words := len(strings.Fields(text)); words == 0 || words > MaxInterpretationWords {
		return "", ErrInvalidText
	}
	return text, nil
}

// ApplicationFeedback is application feedback that passed validation.
type ApplicationFeedback struct {
	Rating int
	Text   string
}

// ValidateApplicationFeedback accepts only +1 or -1 and limits optional text
// by bytes to match the existing API contract.
func ValidateApplicationFeedback(rating int, text string) (ApplicationFeedback, error) {
	if rating != -1 && rating != 1 {
		return ApplicationFeedback{}, ErrInvalidRating
	}
	if len(text) > MaxFeedbackTextBytes {
		return ApplicationFeedback{}, ErrInvalidText
	}
	return ApplicationFeedback{Rating: rating, Text: text}, nil
}

// ValidateEmail trims and validates a plain email address. Display-name forms
// are rejected because persistence stores only the address.
func ValidateEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) < 3 || len(email) > 320 {
		return "", ErrInvalidEmail
	}
	return email, nil
}

// TranslationFeedbackInput is raw feedback about a completed translation.
type TranslationFeedbackInput struct {
	TranslationID   string
	Revision        int32
	Provider        string
	SourceLang      string
	TargetLang      string
	SourceText      string
	TranslationText string
	Assessment      string
}

// TranslationFeedback is normalized feedback safe for persistence.
type TranslationFeedback struct {
	TranslationID   uuid.UUID
	Revision        int32
	Languages       LanguageSelection
	SourceText      string
	TranslationText string
	Assessment      string
}

// ValidateTranslationFeedback validates provider/language metadata, both text
// values, and the like/dislike assessment used by the extension.
func ValidateTranslationFeedback(catalog *Catalog, input TranslationFeedbackInput, maxTextBytes int) (TranslationFeedback, error) {
	if catalog == nil {
		return TranslationFeedback{}, errors.New("language catalog is nil")
	}
	if err := PositiveInt("max text bytes", maxTextBytes); err != nil {
		return TranslationFeedback{}, err
	}
	translationID, err := uuid.Parse(strings.TrimSpace(input.TranslationID))
	if err != nil || translationID == uuid.Nil {
		return TranslationFeedback{}, ErrInvalidTranslationID
	}
	if input.Revision <= 0 {
		return TranslationFeedback{}, ErrInvalidRevision
	}

	languages, err := catalog.ValidateSelection(input.Provider, input.SourceLang, input.TargetLang)
	if err != nil {
		return TranslationFeedback{}, err
	}
	if input.SourceText == "" || len(input.SourceText) > maxTextBytes ||
		input.TranslationText == "" || len(input.TranslationText) > maxTextBytes {
		return TranslationFeedback{}, ErrInvalidText
	}

	assessment := strings.ToLower(strings.TrimSpace(input.Assessment))
	if assessment != "like" && assessment != "dislike" {
		return TranslationFeedback{}, ErrInvalidAssessment
	}

	return TranslationFeedback{
		TranslationID:   translationID,
		Revision:        input.Revision,
		Languages:       languages,
		SourceText:      input.SourceText,
		TranslationText: input.TranslationText,
		Assessment:      assessment,
	}, nil
}
