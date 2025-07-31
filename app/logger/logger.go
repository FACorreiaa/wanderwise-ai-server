package logger

import (
	"sync"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Log      *zap.Logger
	onceInit sync.Once
)

func Init(level zapcore.Level, meta ...zap.Field) error {
	onceInit.Do(func() {
		instance := zap.Must(configure(level).Build())

		instance = instance.With(meta...)
		instance = instance.With(zap.String("line", "42"))

		Log = zap.New(instance.Core(), zap.AddCaller())
	})

	if Log == nil {
		return errors.New("logger not initialized")
	}

	return nil
}

func configure(level zapcore.Level) zap.Config {
	encoder := zap.NewProductionEncoderConfig()
	encoder.TimeKey = "timestamp"
	encoder.EncodeTime = zapcore.ISO8601TimeEncoder
	encoder.EncodeLevel = zapcore.CapitalColorLevelEncoder
	encoder.EncodeCaller = zapcore.ShortCallerEncoder
	encoder.EncodeDuration = zapcore.SecondsDurationEncoder
	encoder.EncodeName = zapcore.FullNameEncoder
	encoder.CallerKey = "caller"
	return zap.Config{
		Level:             zap.NewAtomicLevelAt(level),
		Development:       false,
		DisableCaller:     false,
		DisableStacktrace: false,
		Encoding:          "console",
		EncoderConfig:     encoder,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
	}
}
