package log

import (
	"context"
	"fmt"
	"os"

	config "github.com/Sainarasimhan/sample/pkg/cfg"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Log package

// ServiceName - Name to be printed in Log
const (
	ServiceName = "Sample"
	CONSOLE     = "console"
	JSON        = "json"
)

const (
	// DebugLevel logs are typically voluminous, and are usually disabled in
	// production.
	DebugLevel = "Debug"
	// InfoLevel is the default logging priority.
	InfoLevel = "Info"
	// WarnLevel logs are more important than Info, but don't need individual
	// human review.
	WarnLevel = "Warn"
	// ErrorLevel logs are high-priority. If an application is running smoothly,
	// it shouldn't generate any error-level logs.
	ErrorLevel = "Error"
	// FatalLevel logs a message, then calls os.Exit(1).
	FatalLevel = "Fatal"
)

// key for storing log structure in contexts
type key int8

// Cfg -- Log config
type Cfg config.LogCfg

// MsgStruct - Set log context in ctx
type MsgStruct struct {
	Transport string
	ID        string
	Event     string
	// Other params can be added as req
}

// package vars
var logKey key

var (
	defaultSlog = MsgStruct{
		Transport: "",
		ID:        "Sys",
		Event:     "Sys",
	}
)

// Logger - generic logger interface
type Logger interface {
	// Sugarad Log Funcs
	Debug(context.Context, ...interface{})
	Info(context.Context, ...interface{})
	Error(context.Context, ...interface{})
	Fatal(context.Context, ...interface{})
	Debugw(context.Context, ...interface{})
	Infow(context.Context, ...interface{})
	Errorw(context.Context, ...interface{})
	Fatalw(context.Context, ...interface{})
}

// New - Returns Logger interface (powered by Zap)
func New(name string, c config.LogCfg) Logger {
	return NewZapLogger(name, c)
}

// NewZapLogger - Returns Zap Logger - Use for performant effective logging
func NewZapLogger(name string, c config.LogCfg) *zapLogger {

	localCfg := Cfg(c)
	l, err := localCfg.getZapCfg().Build() //Build from passed Config
	if err != nil {
		panic(err)
	}
	return &zapLogger{Logger: l.Named(name + "-" + ServiceName)}
}

// NewContext -- returns context with Log struct
func NewContext(ctx context.Context, s *MsgStruct) context.Context {
	return context.WithValue(ctx, logKey, s)
}

// FromContext - Returns log struct from context
func FromContext(ctx context.Context) (*MsgStruct, bool) {
	if ctx != nil {
		s, ok := ctx.Value(logKey).(*MsgStruct)
		return s, ok
	}
	return nil, false
}

// Internal func to get Log Struct from Context, if No log struct available returns default log msg
func getCtxMsg(ctx context.Context) string {
	msg := defaultSlog.String()
	if s, ok := FromContext(ctx); ok {
		msg = s.String()
	}
	return msg
}

// type Implementing Logger Interface
type zapLogger struct {
	*zap.Logger
}

// Default Zap Logger Vars
var (
	// DefaultZapCfg - Default Log config
	DefaultZapCfg = zap.Config{
		Level:            zap.NewAtomicLevelAt(zap.InfoLevel),
		Encoding:         "console",
		OutputPaths:      []string{os.Stdout.Name()},
		ErrorOutputPaths: []string{os.Stderr.Name()},
		//InitialFields:    map[string]interface{}{"Name": "SampleSvc"},
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "TS",
			LevelKey:       "Level",
			NameKey:        "Mod",
			MessageKey:     "Msg",
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
		},
	}
)

// Debug - Adds debug data with request context
func (z *zapLogger) Debug(ctx context.Context, args ...interface{}) {
	z.Sugar().Debug(getCtxMsg(ctx), args)
}

// Info - Adds Info level Message with request context
func (z *zapLogger) Info(ctx context.Context, args ...interface{}) {
	z.Sugar().Info(getCtxMsg(ctx), args)
}

// Error - Adds error details with request context
func (z *zapLogger) Error(ctx context.Context, args ...interface{}) {
	z.Sugar().Error(getCtxMsg(ctx), args)
}

// Fatal - Adds error details with request context and exits
func (z *zapLogger) Fatal(ctx context.Context, args ...interface{}) {
	z.Sugar().Fatal(getCtxMsg(ctx), args)
}

// Debugw - Adds debug data as key value pairs, with request context
func (z *zapLogger) Debugw(ctx context.Context, keyvalues ...interface{}) {
	z.Sugar().Debugw(getCtxMsg(ctx), keyvalues...)
}

// Infow - Adds Info as key value pairs, with request context
func (z *zapLogger) Infow(ctx context.Context, keyvalues ...interface{}) {
	z.Sugar().Infow(getCtxMsg(ctx), keyvalues...)
}

// Errorw - Adds Error details key value pairs, with request context
func (z *zapLogger) Errorw(ctx context.Context, keyvalues ...interface{}) {
	z.Sugar().Errorw(getCtxMsg(ctx), keyvalues...)
}

// Fatalw- Adds Error details key value pairs, with request context and exits
func (z *zapLogger) Fatalw(ctx context.Context, keyvalues ...interface{}) {
	z.Sugar().Fatalw(getCtxMsg(ctx), keyvalues...)
}

// Use Log Functions with Zap fields, which will be performant effective as type conversion is avoided.
// Debugf - Log zap fields, with request context
func (z *zapLogger) Debugf(ctx context.Context, msg string, fields ...zap.Field) {
	z.Logger.Debug(getCtxMsg(ctx), fields...)
}

// Infof - Log zap fields, with request context
func (z *zapLogger) Infof(ctx context.Context, msg string, fields ...zap.Field) {
	z.Logger.Info(getCtxMsg(ctx), fields...)
}

// Errorf - Log zap fields, with request context
func (z *zapLogger) Errorf(ctx context.Context, msg string, fields ...zap.Field) {
	z.Logger.Error(getCtxMsg(ctx), fields...)
}

// Fatalf- Log zap fields, with request context and exits
func (z *zapLogger) Fatalf(ctx context.Context, msg string, fields ...zap.Field) {
	z.Logger.Fatal(getCtxMsg(ctx), fields...)
}

// Format Funcs
func (c *Cfg) getZapCfg() zap.Config {

	zapCfg := DefaultZapCfg
	switch c.Level {
	case DebugLevel:
		zapCfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case InfoLevel:
		zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case WarnLevel:
		zapCfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case ErrorLevel:
		zapCfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	case FatalLevel:
		zapCfg.Level = zap.NewAtomicLevelAt(zap.FatalLevel)
	}

	if c.Format != "" {
		zapCfg.Encoding = c.Format
	}
	if CONSOLE == c.Format {
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	return zapCfg

}

func (s *MsgStruct) zapFields() []zap.Field {
	return []zap.Field{
		zap.String("Trpt", s.Transport),
		zap.String("ID", s.ID),
		zap.String("Evt", s.Event),
	}
}

func (s *MsgStruct) String() string {
	if s.Transport != "" { // Skipping Transport for Internal Logs
		return fmt.Sprintf("Trpt=%s, ID=%s, Evt=%s", s.Transport, s.ID, s.Event)
	}
	return fmt.Sprintf("ID=%s, Evt=%s", s.ID, s.Event)
}
