package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
)

type Logger interface {
	Log(level LogLevel, format string, args ...interface{})
}

type DefaultLogger struct {
	inner *slog.Logger
}

// NewDefaultLogger writes to stdout and the given log file using slog's
// text handler. mode is the minimum level emitted; entries below it are
// dropped by slog itself.
func NewDefaultLogger(mode LogLevel, logFile string) (*DefaultLogger, error) {
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}

	multiWriter := io.MultiWriter(os.Stdout, file)
	handler := slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
		Level: parseLevel(mode),
	})
	return &DefaultLogger{inner: slog.New(handler)}, nil
}

func parseLevel(l LogLevel) slog.Level {
	switch LogLevel(strings.ToUpper(string(l))) {
	case LogLevelDebug:
		return slog.LevelDebug
	case LogLevelWarn:
		return slog.LevelWarn
	case LogLevelError:
		return slog.LevelError
	case LogLevelInfo:
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}

// Log formats the message with fmt.Sprintf, then forwards to slog at the
// matching level. Existing call sites use printf-style format strings; we
// keep that working by formatting eagerly.
func (l *DefaultLogger) Log(level LogLevel, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	switch parseLevel(level) {
	case slog.LevelDebug:
		l.inner.Debug(msg)
	case slog.LevelWarn:
		l.inner.Warn(msg)
	case slog.LevelError:
		l.inner.Error(msg)
	default:
		l.inner.Info(msg)
	}
}
