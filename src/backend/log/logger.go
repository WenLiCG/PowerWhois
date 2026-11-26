package log

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	logger    *zap.Logger
	sugar     *zap.SugaredLogger
	logLevel  zap.AtomicLevel
	logWriter *lumberjack.Logger
	appDev    = os.Getenv("APP_DEV")
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

	logDir := filepath.Join(filepath.Dir(execPath), logDirName)
	if configured := os.Getenv("POWERWHOIS_LOG_DIR"); configured != "" {
		logDir = filepath.Clean(configured)
		if err := os.MkdirAll(logDir, 0700); err != nil {
			fmt.Fprintln(os.Stderr, "cannot create log directory")
			os.Exit(1)
		}
	}
	logWriter = &lumberjack.Logger{
		Filename: filepath.Join(logDir, logFileName),
		MaxSize:  logFileMaxSize,  // 每个日志文件最大尺寸（MB）
		MaxAge:   logFileMaxAge,   // 保留的最大天数
		Compress: logFileCompress, // 是否压缩
	}
	fileWriteSyncer := zapcore.AddSync(logWriter)

	// 创建同步器. The file stays the source for the Web log download; containers also set
	// POWERWHOIS_LOG_STDOUT=true so `docker compose logs typonamer` shows the application log.
	var syncer zapcore.WriteSyncer = fileWriteSyncer
	if strings.EqualFold(appDev, "true") {
		syncer = zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stderr), fileWriteSyncer)
	} else if strings.EqualFold(os.Getenv("POWERWHOIS_LOG_STDOUT"), "true") {
		syncer = zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout), fileWriteSyncer)
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

func logFiles() ([]string, error) {
	dir := filepath.Dir(logWriter.Filename)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		if name != logFileName {
			// lumberjack inserts this timestamp between the base name and extension.
			const layout = "typonamer-2006-01-02T15-04-05.000.log"
			if stamp, err := time.Parse(layout, name); err != nil || stamp.Format(layout) != name {
				continue
			}
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	return paths, nil
}

func WriteZipLogs(out io.Writer) error {
	paths, err := logFiles()
	if err != nil {
		return err
	}
	w := zip.NewWriter(out)
	for _, path := range paths {
		if err := addLogToZip(w, path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return w.Close()
}

func addLogToZip(w *zip.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Method = zip.Deflate
	entry, err := w.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, io.LimitReader(f, info.Size()))
	return err
}

func ResetLogsFile() error {
	if info, err := os.Lstat(logWriter.Filename); err == nil && !info.Mode().IsRegular() {
		return errors.New("current log is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// Rotate under the writer's lock so subsequent writes start at the new file's offset zero.
	if err := logWriter.Rotate(); err != nil {
		return err
	}
	paths, err := logFiles()
	if err != nil {
		return err
	}
	for _, path := range paths {
		if path != logWriter.Filename {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
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
