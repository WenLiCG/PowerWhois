package log

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dromara/carbon/v2"
	"github.com/duke-git/lancet/v2/fileutil"
	"github.com/duke-git/lancet/v2/strutil"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	logDirName      = "logs"          // 日志文件夹名称
	logFileName     = "typonamer.log" // 日志文件名称
	logFileMaxSize  = 20              // 每个日志文件最大尺寸（MB）
	logFileMaxAge   = 30              // 保留的最大天数
	logFileCompress = false           // 是否压缩
)

var (
	logger   *zap.Logger
	sugar    *zap.SugaredLogger
	logLevel zap.AtomicLevel
	execDir  string
	logDir   string
	logFile  string
	appDev   = os.Getenv("APP_DEV")
)

func init() {
	// 创建基础的 encoder 配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:       "time",
		LevelKey:      "level",
		NameKey:       "logger",
		CallerKey:     "caller",
		FunctionKey:   zapcore.OmitKey,
		MessageKey:    "msg",
		StacktraceKey: "stacktrace",
		LineEnding:    zapcore.DefaultLineEnding,
		EncodeLevel:   zapcore.CapitalLevelEncoder,
		EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
		},
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 设置日志级别
	logLevel = zap.NewAtomicLevelAt(zap.InfoLevel)

	// 创建输出格式器
	logEncoder := zapcore.NewConsoleEncoder(encoderConfig)

	// 创建文件输出
	execPath, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}

	execDir = filepath.Dir(execPath)
	logDir = filepath.Join(execDir, logDirName)
	if configured := os.Getenv("POWERWHOIS_LOG_DIR"); configured != "" {
		logDir = filepath.Clean(configured)
		execDir = filepath.Dir(logDir)
		if err := os.MkdirAll(logDir, 0700); err != nil {
			fmt.Fprintln(os.Stderr, "cannot create log directory")
			os.Exit(1)
		}
	}
	logFile = filepath.Join(logDir, logFileName)

	fileWriteSyncer := zapcore.AddSync(&lumberjack.Logger{
		Filename: logFile,         // 日志文件路径
		MaxSize:  logFileMaxSize,  // 每个日志文件最大尺寸（MB）
		MaxAge:   logFileMaxAge,   // 保留的最大天数
		Compress: logFileCompress, // 是否压缩
	})

	// 创建同步器
	var syncer zapcore.WriteSyncer
	if appDev != "" && strings.ToLower(appDev) == "true" {
		// 创建控制台输出
		consoleSyncer := zapcore.AddSync(os.Stderr)

		// 创建多输出
		syncer = zapcore.NewMultiWriteSyncer(consoleSyncer, fileWriteSyncer)
	} else {
		syncer = fileWriteSyncer
	}

	// 创建核心
	core := zapcore.NewCore(logEncoder, syncer, logLevel)

	// 创建 logger
	logger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	sugar = logger.Sugar()
}

func Sync() {
	logger.Sync()
}

func SetLevel(level string) {
	if level != "" {
		switch strutil.SnakeCase(level) {
		case "error":
			logLevel.SetLevel(zapcore.ErrorLevel)
		case "warn":
			logLevel.SetLevel(zapcore.WarnLevel)
		case "info":
			logLevel.SetLevel(zapcore.InfoLevel)
		case "debug":
			logLevel.SetLevel(zapcore.DebugLevel)
		case "off":
			logLevel.SetLevel(zapcore.FatalLevel)
		default:
			logLevel.SetLevel(zapcore.InfoLevel)
		}
	} else {
		logLevel.SetLevel(zapcore.InfoLevel)
	}
}

// GetZipLogsFile compresses the log directory into a zip file.
// It returns the path to the zip file and an error if the compression fails.
func GetZipLogsFile() (string, error) {
	clearZipLogsFile()

	zipFileName := fmt.Sprintf("typonamer_logs_%s.zip", carbon.Now().ToShortDateTimeString())
	destZipFile := filepath.Join(execDir, zipFileName)
	err := fileutil.Zip(logDir, destZipFile)
	if err != nil {
		logger.Error("Error compressing log directory: ", zap.Error(err))
		return "", err
	}

	return destZipFile, nil
}

func clearZipLogsFile() error {
	fileList, err := fileutil.ListFileNames(execDir)
	if err != nil {
		logger.Error("Error listing files in directory: ", zap.Error(err))
		return err
	}
	for _, file := range fileList {
		if strings.HasPrefix(file, "typonamer_logs_") && strings.HasSuffix(file, ".zip") {
			os.Remove(filepath.Join(execDir, file))
		}
	}
	return nil
}

// ResetLogsFile resets the log file by clearing its contents.
// It returns an error if the log file is not initialized or if there is an error clearing the file.
func ResetLogsFile() error {
	if logFile == "" {
		logger.Error("Log file not initialized")
		return errors.New("log file not initialized")
	}

	err := fileutil.ClearFile(logFile)
	if err != nil {
		logger.Error("Error resetting log file: ", zap.Error(err))
		return err
	}

	fileList, err := fileutil.ListFileNames(logDir)
	if err != nil {
		logger.Error("Error listing files in directory: ", zap.Error(err))
		return err
	}

	for _, file := range fileList {
		if file != logFileName {
			if strings.HasSuffix(file, ".log") {
				os.Remove(filepath.Join(logDir, file))
			}
		}
	}

	return nil
}

func Fatal(v ...interface{}) {
	sugar.Fatal(v...)
}

func Error(v ...interface{}) {
	sugar.Error(v...)
}

func Warn(v ...interface{}) {
	sugar.Warn(v...)
}

func Info(v ...interface{}) {
	sugar.Info(v...)
}

func Debug(v ...interface{}) {
	sugar.Debug(v...)
}

func Panic(v ...interface{}) {
	sugar.Panic(v...)
}

func Fatalf(format string, v ...interface{}) {
	sugar.Fatalf(format, v...)
}

func Errorf(format string, v ...interface{}) {
	sugar.Errorf(format, v...)
}

func Warnf(format string, v ...interface{}) {
	sugar.Warnf(format, v...)
}

func Infof(format string, v ...interface{}) {
	sugar.Infof(format, v...)
}

func Debugf(format string, v ...interface{}) {
	sugar.Debugf(format, v...)
}

func Panicf(format string, v ...interface{}) {
	sugar.Panicf(format, v...)
}
