package main

import (
	"context"
	"io"
	"log/slog"
)

// Logger provides structured logging.
type Logger struct {
	l *slog.Logger
}

type logMod string

func NewLogger(file io.Writer, opts *slog.HandlerOptions) (*Logger, error) {
	return &Logger{
		l: slog.New(slog.NewJSONHandler(file, opts)),
	}, nil
}

func (l *Logger) Log(ctx context.Context, module logMod, format string, args ...any) {
	args = append([]any{"mod", module}, args...)
	l.l.Log(ctx, slog.LevelInfo, format, args...)
}

func (l *Logger) Warn(ctx context.Context, module logMod, format string, args ...any) {
	args = append([]any{"mod", module}, args...)
	l.l.Log(ctx, slog.LevelWarn, format, args...)
}
