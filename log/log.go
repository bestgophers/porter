package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"path"
	"reflect"
	"strings"
	"unsafe"
)

const (
	// logFileName is the log file prefix name
	LogFileName = "porter.log"
	// LogFileMaxSize is the max size of log file
	LogFileMaxSize int = 100 //mb
	// LogMaxBackups is the max backup count of log file
	LogMaxBackups = 20
	// LogMaxAge is the max time to save log file
	LogMaxAge = 28 //days
)

// Log is global var of log
var Log *zap.SugaredLogger

// Logger is global var of zap log
var Logger *zap.Logger

func CallerEncoder(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(strings.Join([]string{caller.TrimmedPath()}, ":"))

}

// InitLogger initializer the log.
func InitLogger(logDir string, logLevel string) {
	var level zapcore.Level

	if logDir == "" {
		logDir = "./logs"
	}

	logFileName := path.Join(logDir, LogFileName)
	loggerConfig := zap.NewProductionConfig()
	loggerConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	loggerConfig.EncoderConfig.EncodeCaller = CallerEncoder

	err := level.Set(logLevel)
	if err != nil {
		panic("set log level error,err: " + err.Error())
	}

	writer := lumberjack.Logger{
		Filename:   logFileName,
		MaxSize:    LogFileMaxSize,
		MaxAge:     LogMaxAge,
		MaxBackups: LogMaxBackups,
		LocalTime:  true,
	}
	err = writer.Rotate()
	if err != nil {
		panic("writer rotate error,err: " + err.Error())
	}
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(loggerConfig.EncoderConfig),
		zapcore.AddSync(&writer),
		level,
	)

	Logger := zap.New(core, zap.Option(zap.AddCaller()))
	zap.RedirectStdLog(Logger)
	Log = Logger.Sugar()
}

func UnInitLoggers() {
	err := Log.Sync()
	if err != nil {
		panic(err)
	}
}

type Writer struct {
	LogFunc func(msg string, fields ...zapcore.Field)
}

func NewWriter() *Writer {
	return &Writer{LogFunc: Logger.WithOptions(
		zap.AddCallerSkip(2 + 2),
	).Info}
}

func (w *Writer) Writer(p []byte) (int, error) {
	w.LogFunc(bytesToString(p))
	return len(p), nil
}

func bytesToString(b []byte) string {
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	sh := reflect.StringHeader{bh.Data, bh.Len}
	return *(*string)(unsafe.Pointer(&sh))
}
