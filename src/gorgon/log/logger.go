package log

import (
	"context"
	"fmt"
	std_log "log"
	"log/slog"
	"os"
	"runtime"
	"strconv"
)

type Level string

const (
	INFO    Level = "INFO"
	WARNING Level = "WARNING"
	ERROR   Level = "ERROR"
)

func init() {
	jsonlogfile := "/root/store/gorgon_json.log"
	file, err := os.OpenFile(jsonlogfile, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	_jsonlogger = slog.New(slog.NewJSONHandler(file, nil))
}

var _jsonlogger *slog.Logger

type Logger func(level Level, message string)

var _logger Logger = func(level Level, message string) {
	std_log.Println("gorgon:", level, message)
}

func GetLogger() Logger {
	return _logger
}

func SetLogger(logger Logger) {
	_logger = logger
}

func Log(level Level, format string, args ...interface{}) {
	log(level, format, args...)
}

func Info(format string, args ...interface{}) {
	log(INFO, format, args...)
}

func Warning(format string, args ...interface{}) {
	log(WARNING, format, args...)
}

func Error(format string, args ...interface{}) {
	log(ERROR, format, args...)
}

func log(level Level, format string, args ...interface{}) {
	if _logger == nil {
		return
	}

	msg := fmt.Sprintf(format, args...)
	buffer := []byte(msg)
	if _, file, line, ok := runtime.Caller(2); ok {
		if _jsonlogger != nil {
			_jsonlogger.LogAttrs(context.Background(), toSlogLevel(level), msg,
				slog.String("file", file),
				slog.Int("line", line),
			)
		}
		buffer = append(buffer, "\t @"...)
		buffer = append(buffer, file...)
		buffer = append(buffer, ':')
		buffer = append(buffer, strconv.Itoa(line)...)
	}
	_logger(level, string(buffer))
}

func toSlogLevel(level Level) slog.Level {
	switch level {
	case WARNING:
		return slog.LevelWarn
	case ERROR:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
