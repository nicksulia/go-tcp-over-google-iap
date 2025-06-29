package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
	Fatal(msg string, keysAndValues ...any)
}

type ZapLogger struct {
	logger *zap.SugaredLogger
}

func NewZapLogger(level string) (Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.CallerKey = ""
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	switch level {
	case "debug":
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		return nil, fmt.Errorf("unsupported log level: %s", level)
	}
	z, _ := cfg.Build()
	return &ZapLogger{logger: z.Sugar()}, nil
}

func (z *ZapLogger) Debug(msg string, kv ...any) {
	z.logger.Debugw(msg, kv...)
}

func (z *ZapLogger) Info(msg string, kv ...any) {
	z.logger.Infow(msg, kv...)
}

func (z *ZapLogger) Warn(msg string, kv ...any) {
	z.logger.Warnw(msg, kv...)
}

func (z *ZapLogger) Error(msg string, kv ...any) {
	z.logger.Errorw(msg, kv...)
}

func (z *ZapLogger) Fatal(msg string, kv ...any) {
	z.logger.Fatalw(msg, kv...)
}

var _ Logger = (*ZapLogger)(nil)
