package logger

import (
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(level string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.Encoding = "json"
	cfg.EncoderConfig.TimeKey = "time"
	cfg.EncoderConfig.MessageKey = "message"
	cfg.EncoderConfig.LevelKey = "level"
	cfg.EncoderConfig.CallerKey = "caller"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	lvl := strings.ToLower(level)
	switch lvl {
	case "debug", "info", "warn", "error":
		cfg.Level = zap.NewAtomicLevelAt(parseLevel(lvl))
	default:
		cfg.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}

	return cfg.Build()
}

func parseLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func String(key, value string) zap.Field { return zap.String(key, value) }
func Int(key string, value int) zap.Field { return zap.Int(key, value) }
func Duration(key string, value time.Duration) zap.Field { return zap.Duration(key, value) }
func Error(err error) zap.Field { return zap.Error(err) }
func Int64(key string, value int64) zap.Field { return zap.Int64(key, value) }


