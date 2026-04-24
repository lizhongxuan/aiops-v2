package logging

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger = zap.NewNop()
var sugar = logger.Sugar()

type Config struct {
	LogLevel  string
	LogFormat string
}

// Init configures the global logger based on config settings.
func Init(cfg Config) (*zap.Logger, error) {
	level := parseLevel(cfg.LogLevel)
	encoder := buildEncoder(cfg.LogFormat)
	core := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level)
	l := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	SetLogger(l)
	return l, nil
}

// SetLogger replaces the global logger instance.
func SetLogger(l *zap.Logger) {
	if l == nil {
		l = zap.NewNop()
	}
	logger = l
	sugar = l.Sugar()
}

// L returns the configured logger.
func L() *zap.Logger {
	return logger
}

// S returns the configured sugared logger.
func S() *zap.SugaredLogger {
	return sugar
}

func parseLevel(raw string) zapcore.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return zapcore.DebugLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func buildEncoder(format string) zapcore.Encoder {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "console", "text":
		cfg := zap.NewDevelopmentEncoderConfig()
		cfg.EncodeTime = zapcore.ISO8601TimeEncoder
		return zapcore.NewConsoleEncoder(cfg)
	default:
		cfg := zap.NewProductionEncoderConfig()
		cfg.EncodeTime = zapcore.ISO8601TimeEncoder
		return zapcore.NewJSONEncoder(cfg)
	}
}
