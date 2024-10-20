package logger

import (
	"flag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"io"
	"os"
)

func NewConsoleEncoder(opts ...EncoderConfigOption) zapcore.Encoder {
	cfg := zap.NewDevelopmentEncoderConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return zapcore.NewConsoleEncoder(cfg)
}

func NewJSONEncoder(opts ...EncoderConfigOption) zapcore.Encoder {
	cfg := zap.NewProductionEncoderConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return zapcore.NewJSONEncoder(cfg)
}

func defaultNewEncoder(development bool) func(opts ...EncoderConfigOption) zapcore.Encoder {
	if development {
		return NewConsoleEncoder
	}
	return NewJSONEncoder
}

func defaultLevel(development bool) zapcore.LevelEnabler {
	if development {
		return zapcore.DebugLevel
	}
	return zapcore.InfoLevel
}

func defaultStacktraceLevel(development bool) zapcore.LevelEnabler {
	if development {
		return zapcore.WarnLevel
	}
	return zapcore.ErrorLevel
}

type EncoderConfigOption func(cfg *zapcore.EncoderConfig)

type Option func(*Options)

type Options struct {
	// Development puts the logger in development mode.
	Development bool
	// NewEncoder configures the function used to create a new encoder.
	NewEncoder func(opts ...EncoderConfigOption) zapcore.Encoder
	// Encoder configures the encoder used for logging.
	Encoder zapcore.Encoder
	// Output configures the sink where logs are written.
	Output io.Writer
	// Level configures the minimum enabled logging level.
	Level zapcore.LevelEnabler
	// StacktraceLevel configures the level at which stacktraces are captured.
	StacktraceLevel zapcore.LevelEnabler
	// TimeEncoder configures the encoder used for timestamps.
	TimeEncoder zapcore.TimeEncoder
	// ZapOptions configures additional options for the logger.
	ZapOptions []zap.Option
}

// SetDefaults sets the default values for the logger options based on the
// development mode in case some of the options are not set.
func (o *Options) SetDefaults() {
	if o.Output == nil {
		o.Output = os.Stderr
	}
	if o.NewEncoder == nil {
		o.NewEncoder = defaultNewEncoder(o.Development)
	}
	if o.Level == nil {
		o.Level = defaultLevel(o.Development)
	}
	if o.StacktraceLevel == nil {
		o.StacktraceLevel = defaultStacktraceLevel(o.Development)
	}
	if o.TimeEncoder == nil {
		o.TimeEncoder = zapcore.RFC3339TimeEncoder
	}
	if o.Encoder == nil {
		fn := func(cfg *zapcore.EncoderConfig) {
			cfg.EncodeTime = o.TimeEncoder
		}
		o.Encoder = o.NewEncoder(fn)
	}
}

// BindFlags binds the logger options to the given flag set.
// The following flags are available:
//
//	-log-devel: set logger to development mode
//	-log-level: configure verbosity of logger
//	-log-stacktrace-level: configure verbosity of stacktrace level
//	-log-encoder: log encoder
//	-log-time-encoder: time encoder
func (o *Options) BindFlags(fs *flag.FlagSet) {
	fs.BoolVar(&o.Development, "log-devel", false, "set logger to development mode")
	fs.Var(
		&LogLevelValue{target: &o.Level},
		"log-level",
		"configure verbosity of logger (debug, info, warn, error)",
	)
	fs.Var(
		&LogStacktraceLevelValue{target: &o.StacktraceLevel},
		"log-stacktrace-level",
		"configure verbosity of stacktrace level (info, error, panic)",
	)
	fs.Var(
		&LogNewEncoderValue{target: &o.NewEncoder},
		"log-encoder",
		"log encoder (json or console)",
	)
	fs.Var(
		&LogTimeEncoderValue{target: &o.TimeEncoder},
		"log-time-encoder",
		"time encoder (rfc3339, rfc3339nano, iso8601, millis, nanos)",
	)
}

// New creates a new logger with the given options.
// If no options are provided, the logger is created with the default options related to the
// production mode.
func New(options ...Option) *zap.Logger {
	opts := &Options{}
	for _, opt := range options {
		opt(opts)
	}

	opts.SetDefaults()
	if opts.Development {
		opts.ZapOptions = append(opts.ZapOptions, zap.Development())
	}
	opts.ZapOptions = append(opts.ZapOptions, zap.AddStacktrace(opts.StacktraceLevel))

	sink := zapcore.AddSync(opts.Output)
	opts.ZapOptions = append(opts.ZapOptions, zap.ErrorOutput(sink))

	core := zapcore.NewCore(
		opts.Encoder,
		sink,
		opts.Level,
	)
	return zap.New(core, opts.ZapOptions...)
}

// UseOptions returns an option that sets the logger options.
func UseOptions(in *Options) Option {
	return func(out *Options) {
		*out = *in
	}
}

// WithDevelopment returns an option that puts the logger in development mode.
func WithDevelopment() Option {
	return func(o *Options) {
		o.Development = true
	}
}
