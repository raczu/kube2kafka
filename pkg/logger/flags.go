package logger

import (
	"fmt"
	"go.uber.org/zap/zapcore"
	"strings"
)

type LogLevelValue struct {
	target *zapcore.LevelEnabler
	value  string
}

func (ll *LogLevelValue) Set(value string) error {
	normalized := strings.ToLower(value)
	switch normalized {
	case "debug":
		*ll.target = zapcore.DebugLevel
	case "info":
		*ll.target = zapcore.InfoLevel
	case "warn":
		*ll.target = zapcore.WarnLevel
	case "error":
		*ll.target = zapcore.ErrorLevel
	default:
		return fmt.Errorf("unknown log level: \"%s\"", value)
	}
	return nil
}

func (ll *LogLevelValue) String() string {
	return ll.value
}

type LogStacktraceLevelValue struct {
	target *zapcore.LevelEnabler
	value  string
}

func (lsl *LogStacktraceLevelValue) Set(value string) error {
	normalized := strings.ToLower(value)
	switch normalized {
	case "info":
		*lsl.target = zapcore.InfoLevel
	case "error":
		*lsl.target = zapcore.ErrorLevel
	case "panic":
		*lsl.target = zapcore.PanicLevel
	default:
		return fmt.Errorf("unknown stacktrace level: \"%s\"", value)
	}
	return nil
}

func (lsl *LogStacktraceLevelValue) String() string {
	return lsl.value
}

type LogNewEncoderValue struct {
	target *func(opts ...EncoderConfigOption) zapcore.Encoder
	value  string
}

func (le *LogNewEncoderValue) Set(value string) error {
	normalized := strings.ToLower(value)
	switch normalized {
	case "json":
		*le.target = NewJSONEncoder
	case "console":
		*le.target = NewConsoleEncoder
	default:
		return fmt.Errorf("unknown encoder: \"%s\"", value)
	}
	le.value = normalized
	return nil
}

func (le *LogNewEncoderValue) String() string {
	return le.value
}

type LogTimeEncoderValue struct {
	target *zapcore.TimeEncoder
	value  string
}

func (lte *LogTimeEncoderValue) Set(value string) error {
	normalized := strings.ToLower(value)
	switch normalized {
	case "rfc3339":
		*lte.target = zapcore.RFC3339TimeEncoder
	case "rfc3339nano":
		*lte.target = zapcore.RFC3339NanoTimeEncoder
	case "iso8601":
		*lte.target = zapcore.ISO8601TimeEncoder
	case "millis":
		*lte.target = zapcore.EpochMillisTimeEncoder
	case "nanos":
		*lte.target = zapcore.EpochNanosTimeEncoder
	default:
		return fmt.Errorf("unknown time encoder: \"%s\"", value)
	}
	return nil
}

func (lte *LogTimeEncoderValue) String() string {
	return lte.value
}
