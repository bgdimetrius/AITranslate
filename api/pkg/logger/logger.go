// Package logger configures the application's structured logger.
package logger

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a production JSON logger that writes every enabled level to
// stdout. level is the minimum level to emit (debug, info, warn, error, etc.).
func New(level string) (*zap.Logger, error) {
	if strings.TrimSpace(level) == "" {
		level = zap.InfoLevel.String()
	}

	var minimumLevel zapcore.Level
	if err := minimumLevel.UnmarshalText([]byte(strings.ToLower(strings.TrimSpace(level)))); err != nil {
		return nil, fmt.Errorf("parse log level %q: %w", level, err)
	}

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(minimumLevel)
	config.OutputPaths = []string{"stdout"}
	// Zap reports its own internal write failures here. Keep those on the same
	// stream too, so the process never intentionally writes logs to stderr.
	config.ErrorOutputPaths = []string{"stdout"}
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder

	return config.Build()
}
