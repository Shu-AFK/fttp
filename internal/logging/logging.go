package logging

import (
	"fmt"
	"io"
	"log"
	"os"
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
	logMode LogLevel
	logger  *log.Logger
}

func NewDefaultLogger(mode LogLevel, logFile string) (*DefaultLogger, error) {
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}

	multiWriter := io.MultiWriter(os.Stdout, file)
	logger := log.New(multiWriter, "", log.LstdFlags)

	return &DefaultLogger{
		logMode: mode,
		logger:  logger,
	}, nil
}

func (l *DefaultLogger) Log(level LogLevel, format string, args ...interface{}) {
	logLevels := map[LogLevel]int{
		LogLevelDebug: 1,
		LogLevelInfo:  2,
		LogLevelWarn:  3,
		LogLevelError: 4,
	}

	currentLevel := logLevels[l.logMode]
	messageLevel := logLevels[level]

	if messageLevel >= currentLevel {
		l.logger.Printf("[%s] %s", level, fmt.Sprintf(format, args...))
	}
}
