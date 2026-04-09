package logger

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	log         *zap.Logger
	sugar       *zap.SugaredLogger
	logMutex    sync.RWMutex
	once        sync.Once
	initialized bool
)

// Init 初始化日志 (线程安全)
// 注意：多次调用 Init 只有第一次调用会真正初始化，后续调用会被忽略
func Init(level string, development bool) error {
	var initErr error
	once.Do(func() {
		initErr = doInit(level, development)
	})
	return initErr
}

// doInit 执行实际的日志初始化
func doInit(level string, development bool) error {
	// 解析日志级别
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	// 配置
	config := zap.Config{
		Level:       zap.NewAtomicLevelAt(zapLevel),
		Development: development,
		Encoding:    "console",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "T",
			LevelKey:       "L",
			NameKey:        "N",
			CallerKey:      "C",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "M",
			StacktraceKey:  "S",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalColorLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	// 创建 logger
	newLog, err := config.Build(zap.AddCallerSkip(1))
	if err != nil {
		return err
	}

	// 获取写锁来更新全局变量
	logMutex.Lock()
	log = newLog
	sugar = log.Sugar()
	initialized = true
	logMutex.Unlock()

	return nil
}

// L 获取 logger (线程安全)
func L() *zap.Logger {
	logMutex.RLock()
	if initialized {
		l := log
		logMutex.RUnlock()
		return l
	}
	logMutex.RUnlock()

	// 如果未初始化，使用默认配置
	_ = Init("info", false)

	logMutex.RLock()
	defer logMutex.RUnlock()
	return log
}

// S 获取 sugared logger (线程安全)
func S() *zap.SugaredLogger {
	logMutex.RLock()
	if initialized {
		s := sugar
		logMutex.RUnlock()
		return s
	}
	logMutex.RUnlock()

	// 如果未初始化，使用默认配置
	_ = Init("info", false)

	logMutex.RLock()
	defer logMutex.RUnlock()
	return sugar
}

// Sync 同步日志 (线程安全)
func Sync() error {
	logMutex.RLock()
	defer logMutex.RUnlock()
	if log != nil {
		return log.Sync()
	}
	return nil
}

// With 创建带字段的 logger
func With(fields ...zap.Field) *zap.Logger {
	return L().With(fields...)
}

// Debug 调试日志
func Debug(msg string, fields ...zap.Field) {
	L().Debug(msg, fields...)
}

// Info 信息日志
func Info(msg string, fields ...zap.Field) {
	L().Info(msg, fields...)
}

// Warn 警告日志
func Warn(msg string, fields ...zap.Field) {
	L().Warn(msg, fields...)
}

// Error 错误日志
func Error(msg string, fields ...zap.Field) {
	L().Error(msg, fields...)
}

// Fatal 致命错误日志
func Fatal(msg string, fields ...zap.Field) {
	L().Fatal(msg, fields...)
	os.Exit(1)
}
