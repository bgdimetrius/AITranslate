package analytics

import (
	"context"
	"errors"
	"testing"

	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"github.com/bgdimetrius/AITranslate/pkg/validation"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type analyticsDBStub struct {
	valid           bool
	validateErr     error
	feedbackErr     error
	leadErr         error
	translationErr  error
	feedbackRows    []queries.SaveApplicationFeedbackParams
	leadRows        []queries.SaveFeedbackLeadParams
	translationRows []queries.SaveTranslationFeedbackParams
}

func (db *analyticsDBStub) ValidateUserId(context.Context, int64) (bool, error) {
	return db.valid, db.validateErr
}

func (db *analyticsDBStub) SaveApplicationFeedback(_ context.Context, params queries.SaveApplicationFeedbackParams) error {
	db.feedbackRows = append(db.feedbackRows, params)
	return db.feedbackErr
}

func (db *analyticsDBStub) SaveFeedbackLead(_ context.Context, params queries.SaveFeedbackLeadParams) error {
	db.leadRows = append(db.leadRows, params)
	return db.leadErr
}

func (db *analyticsDBStub) SaveTranslationFeedback(_ context.Context, params queries.SaveTranslationFeedbackParams) error {
	db.translationRows = append(db.translationRows, params)
	return db.translationErr
}

// TestAnalyticsServicePersistsValidatedValues verifies feedback and normalized
// email values are associated with the authenticated installation.
func TestAnalyticsServicePersistsValidatedValues(t *testing.T) {
	db := &analyticsDBStub{valid: true}
	service := NewAnalyticsService(db)

	if err := service.SaveApplicationFeedback(context.Background(), zap.NewNop(), 42, rating{Rating: -1, Text: "not useful"}); err != nil {
		t.Fatalf("SaveApplicationFeedback() error = %v", err)
	}
	if len(db.feedbackRows) != 1 || db.feedbackRows[0] != (queries.SaveApplicationFeedbackParams{InstallationID: 42, Rating: -1, FeedbackText: "not useful"}) {
		t.Fatalf("stored feedback = %#v; want installation 42 rating -1", db.feedbackRows)
	}

	if err := service.SaveFeedbackLead(context.Background(), zap.NewNop(), 42, "  reader@example.com  "); err != nil {
		t.Fatalf("SaveFeedbackLead() error = %v", err)
	}
	if len(db.leadRows) != 1 || db.leadRows[0] != (queries.SaveFeedbackLeadParams{InstallationID: 42, Email: "reader@example.com"}) {
		t.Fatalf("stored lead = %#v; want normalized email for installation 42", db.leadRows)
	}

	translationFeedback := validation.TranslationFeedback{
		TranslationID:   uuid.MustParse("8bdf29a7-54d2-48ee-bd2b-c0ae79af6831"),
		Revision:        2,
		Languages:       validation.LanguageSelection{Provider: validation.ProviderChatGPT, Source: "English", Target: "Spanish"},
		SourceText:      "hello",
		TranslationText: "hola",
		Assessment:      "like",
	}
	if err := service.SaveTranslationFeedback(context.Background(), zap.NewNop(), 42, translationFeedback); err != nil {
		t.Fatalf("SaveTranslationFeedback() error = %v", err)
	}
	wantTranslation := queries.SaveTranslationFeedbackParams{
		InstallationID:  42,
		TranslationID:   translationFeedback.TranslationID,
		Revision:        2,
		Provider:        "chatgpt",
		SourceLang:      "English",
		TargetLang:      "Spanish",
		SourceText:      "hello",
		TranslationText: "hola",
		Assessment:      "like",
	}
	if len(db.translationRows) != 1 || db.translationRows[0] != wantTranslation {
		t.Fatalf("stored translation feedback = %#v; want %#v", db.translationRows, wantTranslation)
	}
}

// TestAnalyticsServiceReturnsPersistenceErrors verifies database write errors
// remain visible to the HTTP adapter.
func TestAnalyticsServiceReturnsPersistenceErrors(t *testing.T) {
	feedbackErr := errors.New("feedback unavailable")
	leadErr := errors.New("lead unavailable")
	translationErr := errors.New("translation feedback unavailable")
	db := &analyticsDBStub{valid: true, feedbackErr: feedbackErr, leadErr: leadErr, translationErr: translationErr}
	service := NewAnalyticsService(db)

	if err := service.SaveApplicationFeedback(context.Background(), zap.NewNop(), 42, rating{Rating: 1}); !errors.Is(err, feedbackErr) {
		t.Fatalf("SaveApplicationFeedback() error = %v; want %v", err, feedbackErr)
	}
	if err := service.SaveFeedbackLead(context.Background(), zap.NewNop(), 42, "reader@example.com"); !errors.Is(err, leadErr) {
		t.Fatalf("SaveFeedbackLead() error = %v; want %v", err, leadErr)
	}
	if err := service.SaveTranslationFeedback(context.Background(), zap.NewNop(), 42, validTranslationFeedback()); !errors.Is(err, translationErr) {
		t.Fatalf("SaveTranslationFeedback() error = %v; want %v", err, translationErr)
	}
}

// TestAnalyticsServicePreconditions covers required dependencies, active users,
// and the validated feedback and email contract.
func TestAnalyticsServicePreconditions(t *testing.T) {
	tests := []struct {
		name string
		call func()
	}{
		{name: "nil feedback context", call: func() {
			NewAnalyticsService(&analyticsDBStub{valid: true}).SaveApplicationFeedback(nil, zap.NewNop(), 42, rating{Rating: 1})
		}},
		{name: "nil feedback logger", call: func() {
			NewAnalyticsService(&analyticsDBStub{valid: true}).SaveApplicationFeedback(context.Background(), nil, 42, rating{Rating: 1})
		}},
		{name: "invalid feedback user", call: func() {
			NewAnalyticsService(&analyticsDBStub{valid: true}).SaveApplicationFeedback(context.Background(), zap.NewNop(), 0, rating{Rating: 1})
		}},
		{name: "unknown feedback user", call: func() {
			NewAnalyticsService(&analyticsDBStub{}).SaveApplicationFeedback(context.Background(), zap.NewNop(), 42, rating{Rating: 1})
		}},
		{name: "feedback user validation failure", call: func() {
			NewAnalyticsService(&analyticsDBStub{validateErr: errors.New("database unavailable")}).SaveApplicationFeedback(context.Background(), zap.NewNop(), 42, rating{Rating: 1})
		}},
		{name: "invalid rating", call: func() {
			NewAnalyticsService(&analyticsDBStub{valid: true}).SaveApplicationFeedback(context.Background(), zap.NewNop(), 42, rating{})
		}},
		{name: "invalid lead email", call: func() {
			NewAnalyticsService(&analyticsDBStub{valid: true}).SaveFeedbackLead(context.Background(), zap.NewNop(), 42, "not-an-email")
		}},
		{name: "invalid translation feedback user", call: func() {
			NewAnalyticsService(&analyticsDBStub{valid: true}).SaveTranslationFeedback(context.Background(), zap.NewNop(), 0, validTranslationFeedback())
		}},
		{name: "unknown translation feedback user", call: func() {
			NewAnalyticsService(&analyticsDBStub{}).SaveTranslationFeedback(context.Background(), zap.NewNop(), 42, validTranslationFeedback())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !analyticsPanics(test.call) {
				t.Fatal("analytics service did not panic")
			}
		})
	}
}

func validTranslationFeedback() validation.TranslationFeedback {
	return validation.TranslationFeedback{
		TranslationID:   uuid.MustParse("8bdf29a7-54d2-48ee-bd2b-c0ae79af6831"),
		Revision:        1,
		Languages:       validation.LanguageSelection{Provider: validation.ProviderChatGPT, Source: "English", Target: "Spanish"},
		SourceText:      "hello",
		TranslationText: "hola",
		Assessment:      "like",
	}
}
