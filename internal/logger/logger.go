package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

type Options struct {
	Level  string
	Format string
	Out    io.Writer
}

func New(opts Options) *slog.Logger {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	level := parseLevel(opts.Level)
	hopts := &slog.HandlerOptions{Level: level, AddSource: false}

	var h slog.Handler
	switch strings.ToLower(opts.Format) {
	case "text":
		h = slog.NewTextHandler(opts.Out, hopts)
	default:
		h = slog.NewJSONHandler(opts.Out, hopts)
	}
	return slog.New(h)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
