package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.Logger

// Init 初始化日誌系統
func Init(isDevelopment bool) error {
	var config zap.Config

	if isDevelopment {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		config = zap.NewProductionConfig()
	}

	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stderr"}

	var err error
	Log, err = config.Build()
	if err != nil {
		return err
	}

	return nil
}

// Sync 同步日誌
func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}

// GetLogger 獲取日誌實例
func GetLogger() *zap.Logger {
	if Log == nil {
		// 如果沒有初始化，使用預設配置
		logger, _ := zap.NewProduction()
		return logger
	}
	return Log
}

// 便捷函數
func Info(msg string, fields ...zap.Field) {
	GetLogger().Info(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	GetLogger().Error(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	GetLogger().Warn(msg, fields...)
}

func Debug(msg string, fields ...zap.Field) {
	GetLogger().Debug(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	GetLogger().Fatal(msg, fields...)
}

// InitDefault 使用環境變數初始化
func InitDefault() error {
	isDev := os.Getenv("ENVIRONMENT") == "development"
	return Init(isDev)
}
