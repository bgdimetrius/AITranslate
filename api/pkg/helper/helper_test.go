package helper

import (
	"context"
	"errors"
	"testing"

	"github.com/bgdimetrius/AITranslate/pkg"
	"github.com/bgdimetrius/AITranslate/pkg/db/queries"
	"go.uber.org/zap"
)

type usageDBStub struct {
	valid       bool
	validateErr error
	recordErr   error
	records     []queries.RecordDailyUsageParams
}

func (db *usageDBStub) ValidateUserId(context.Context, int64) (bool, error) {
	return db.valid, db.validateErr
}

func (db *usageDBStub) RecordDailyUsage(_ context.Context, params queries.RecordDailyUsageParams) error {
	db.records = append(db.records, params)
	return db.recordErr
}

// TestRecordDailyUsage verifies all externally observable counters are passed
// to persistence and a write failure remains best effort.
func TestRecordDailyUsage(t *testing.T) {
	usage := &pkg.Usage{
		TranslationRequests:    1,
		InterpretationRequests: 2,
		FailedRequests:         3,
		InputCharacters:        4,
		InputTokens:            5,
		OutputTokens:           6,
	}
	for _, test := range []struct {
		name      string
		recordErr error
	}{
		{name: "persists usage"},
		{name: "persistence failure remains best effort", recordErr: errors.New("database unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := &usageDBStub{valid: true, recordErr: test.recordErr}
			RecordDailyUsage(context.Background(), zap.NewNop(), db, 42, usage)
			if len(db.records) != 1 {
				t.Fatalf("usage records = %d; want 1", len(db.records))
			}
			want := queries.RecordDailyUsageParams{
				InstallationID:         42,
				TranslationRequests:    1,
				InterpretationRequests: 2,
				FailedRequests:         3,
				InputCharacters:        4,
				InputTokens:            5,
				OutputTokens:           6,
			}
			if db.records[0] != want {
				t.Fatalf("recorded usage = %#v; want %#v", db.records[0], want)
			}
		})
	}
}

// TestRecordDailyUsagePreconditions covers the documented dependency,
// installation, validation, and non-negative counter requirements.
func TestRecordDailyUsagePreconditions(t *testing.T) {
	validUsage := &pkg.Usage{}
	tests := []struct {
		name  string
		ctx   context.Context
		log   *zap.Logger
		db    DbHandler
		user  int
		usage *pkg.Usage
	}{
		{name: "nil context", log: zap.NewNop(), db: &usageDBStub{valid: true}, user: 42, usage: validUsage},
		{name: "nil logger", ctx: context.Background(), db: &usageDBStub{valid: true}, user: 42, usage: validUsage},
		{name: "nil database", ctx: context.Background(), log: zap.NewNop(), user: 42, usage: validUsage},
		{name: "invalid user ID", ctx: context.Background(), log: zap.NewNop(), db: &usageDBStub{valid: true}, usage: validUsage},
		{name: "unknown user", ctx: context.Background(), log: zap.NewNop(), db: &usageDBStub{}, user: 42, usage: validUsage},
		{name: "user validation failure", ctx: context.Background(), log: zap.NewNop(), db: &usageDBStub{validateErr: errors.New("database unavailable")}, user: 42, usage: validUsage},
		{name: "nil usage", ctx: context.Background(), log: zap.NewNop(), db: &usageDBStub{valid: true}, user: 42},
		{name: "negative translation requests", ctx: context.Background(), log: zap.NewNop(), db: &usageDBStub{valid: true}, user: 42, usage: &pkg.Usage{TranslationRequests: -1}},
		{name: "negative interpretation requests", ctx: context.Background(), log: zap.NewNop(), db: &usageDBStub{valid: true}, user: 42, usage: &pkg.Usage{InterpretationRequests: -1}},
		{name: "negative failed requests", ctx: context.Background(), log: zap.NewNop(), db: &usageDBStub{valid: true}, user: 42, usage: &pkg.Usage{FailedRequests: -1}},
		{name: "negative input characters", ctx: context.Background(), log: zap.NewNop(), db: &usageDBStub{valid: true}, user: 42, usage: &pkg.Usage{InputCharacters: -1}},
		{name: "negative input tokens", ctx: context.Background(), log: zap.NewNop(), db: &usageDBStub{valid: true}, user: 42, usage: &pkg.Usage{InputTokens: -1}},
		{name: "negative output tokens", ctx: context.Background(), log: zap.NewNop(), db: &usageDBStub{valid: true}, user: 42, usage: &pkg.Usage{OutputTokens: -1}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !helperPanics(func() { RecordDailyUsage(test.ctx, test.log, test.db, test.user, test.usage) }) {
				t.Fatal("RecordDailyUsage() did not panic")
			}
		})
	}
}

func helperPanics(fn func()) (didPanic bool) {
	defer func() { didPanic = recover() != nil }()
	fn()
	return false
}
