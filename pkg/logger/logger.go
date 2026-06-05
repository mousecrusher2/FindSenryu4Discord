package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

var defaultLogger *slog.Logger

// Config holds logger configuration
type Config struct {
	Level  string // debug, info, warn, error
	Format string // json, text
	Output io.Writer
}

// Init initializes the global logger with the given configuration
func Init(cfg Config) {
	level := parseLevel(cfg.Level)

	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: level == slog.LevelDebug,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if len(groups) == 0 && attr.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return attr
		},
	}

	priorityOutput := &journalPriorityWriter{output: output}
	var handler slog.Handler
	if strings.ToLower(cfg.Format) == "json" {
		handler = slog.NewJSONHandler(priorityOutput, opts)
	} else {
		handler = slog.NewTextHandler(priorityOutput, opts)
	}
	handler = &journalPriorityHandler{
		handler: handler,
		writer:  priorityOutput,
		mu:      &sync.Mutex{},
	}

	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
}

type journalPriorityHandler struct {
	handler slog.Handler
	writer  *journalPriorityWriter
	mu      *sync.Mutex
}

func (h *journalPriorityHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *journalPriorityHandler) Handle(ctx context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.writer.prefix = journalPriorityPrefix(record.Level)
	defer func() {
		h.writer.prefix = ""
	}()

	return h.handler.Handle(ctx, record)
}

func (h *journalPriorityHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &journalPriorityHandler{
		handler: h.handler.WithAttrs(attrs),
		writer:  h.writer,
		mu:      h.mu,
	}
}

func (h *journalPriorityHandler) WithGroup(name string) slog.Handler {
	return &journalPriorityHandler{
		handler: h.handler.WithGroup(name),
		writer:  h.writer,
		mu:      h.mu,
	}
}

type journalPriorityWriter struct {
	output io.Writer
	prefix string
}

func (w *journalPriorityWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.prefix == "" {
		return w.output.Write(p)
	}

	out := make([]byte, 0, len(p)+len(w.prefix))
	lineStart := true
	for _, b := range p {
		if lineStart {
			out = append(out, w.prefix...)
			lineStart = false
		}
		out = append(out, b)
		if b == '\n' {
			lineStart = true
		}
	}

	if _, err := w.output.Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}

// systemd/journald parses leading "<n>" syslog priority prefixes on stdout/stderr.
func journalPriorityPrefix(level slog.Level) string {
	switch {
	case level < slog.LevelInfo:
		return "<7>" // debug
	case level < slog.LevelWarn:
		return "<6>" // info
	case level < slog.LevelError:
		return "<4>" // warning
	default:
		return "<3>" // error
	}
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Logger returns the default logger
func Logger() *slog.Logger {
	if defaultLogger == nil {
		Init(Config{Level: "info", Format: "text"})
	}
	return defaultLogger
}

// Debug logs at debug level
func Debug(msg string, args ...any) {
	Logger().Debug(msg, args...)
}

// Info logs at info level
func Info(msg string, args ...any) {
	Logger().Info(msg, args...)
}

// Warn logs at warn level
func Warn(msg string, args ...any) {
	Logger().Warn(msg, args...)
}

// Error logs at error level
func Error(msg string, args ...any) {
	Logger().Error(msg, args...)
}
