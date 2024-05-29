// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package log

import (
	"context"
	"flag"
	"os"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	"go.elastic.co/apm/module/apmzap/v2"
	"go.elastic.co/apm/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/klog/v2"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/elastic/cloud-on-k8s/v2/pkg/about"
	"github.com/elastic/cloud-on-k8s/v2/pkg/dev"
)

const (
	EcsVersion     = "1.4.0"
	EcsServiceType = "eck"
	FlagName       = "log-verbosity"

	testLogLevelEnvVar = "ECK_TEST_LOG_LEVEL"
)

var Log = crlog.Log

func init() {
	// Introduced mainly as a workaround for a controller-runtime bug.
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1359#issuecomment-767413330
	// However, it is still useful in general to adjust the log level during test runs.
	if logLevel, err := strconv.Atoi(os.Getenv(testLogLevelEnvVar)); err == nil {
		setLogger(&logLevel)
	}
}

var verbosity = flag.Int(FlagName, 0, "Verbosity level of logs (-2=Error, -1=Warn, 0=Info, >0=Debug)")

// BindFlags attaches logging flags to the given flag set.
func BindFlags(flags *pflag.FlagSet) {
	flags.AddGoFlag(flag.Lookup("log-verbosity"))
}

// InitLogger initializes the global logger informed by the value of log-verbosity flag.
func InitLogger() {
	setLogger(verbosity)
}

// ChangeVerbosity replaces the global logger with a new logger set to the specified verbosity level.
// Verbosity levels from 2 are custom levels that increase the verbosity as the value increases.
// Standard levels are as follows:
// level | Zap level | name
// -------------------------
// 1     | -1        | Debug
// 0     |  0        | Info
// -1    |  1        | Warn
// -2    |  2        | Error
func ChangeVerbosity(v int) {
	setLogger(&v)
}

func setLogger(v *int) {
	zapLevel := determineLogLevel(v)

	// if the Zap custom level is less than debug (verbosity level 2 and above) set the klog level to the same level
	if zapLevel.Level() < zap.DebugLevel {
		flagset := flag.NewFlagSet("", flag.ContinueOnError)
		klog.InitFlags(flagset)
		_ = flagset.Set("v", strconv.Itoa(int(zapLevel.Level())*-1))
	}

	// send error logs to APM
	tracing := zap.WrapCore((&apmzap.Core{}).WrapCore)

	opts := []zap.Option{zap.Fields(
		zap.String("service.version", getVersionString()),
	), tracing}

	var encoder zapcore.Encoder
	if dev.Enabled {
		encoderConf := zap.NewDevelopmentEncoderConfig()
		encoderConf.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConf)
	} else {
		encoderConf := zap.NewProductionEncoderConfig()
		encoderConf.MessageKey = "message"
		encoderConf.TimeKey = "@timestamp"
		encoderConf.LevelKey = "log.level"
		encoderConf.NameKey = "log.logger"
		encoderConf.StacktraceKey = "error.stack_trace"
		encoderConf.EncodeTime = zapcore.ISO8601TimeEncoder
		encoder = zapcore.NewJSONEncoder(encoderConf)
		opts = append(opts,
			zap.Fields(
				zap.String("service.type", EcsServiceType),
				zap.String("ecs.version", EcsVersion),
			))
	}

	stackTraceLevel := zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	crlog.SetLogger(crzap.New(func(o *crzap.Options) {
		o.DestWriter = os.Stderr
		o.Development = dev.Enabled
		o.Level = &zapLevel
		o.StacktraceLevel = &stackTraceLevel
		o.Encoder = encoder
		o.ZapOpts = opts
	}))
}

func determineLogLevel(v *int) zap.AtomicLevel {
	switch {
	case v != nil && *v > -3:
		return zap.NewAtomicLevelAt(zapcore.Level(*v * -1))
	case dev.Enabled:
		return zap.NewAtomicLevelAt(zapcore.DebugLevel)
	default:
		return zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}
}

func getVersionString() string {
	buildInfo := about.GetBuildInfo()
	return buildInfo.VersionString()
}

func NewFromContext(ctx context.Context) logr.Logger {
	traceContextFields := TraceContextKV(ctx)
	return crlog.Log.WithValues(traceContextFields...)
}

// TraceContextKV returns logger key-values for the current trace context.
func TraceContextKV(ctx context.Context) []interface{} {
	tx := apm.TransactionFromContext(ctx)
	if tx == nil {
		return nil
	}

	traceCtx := tx.TraceContext()
	fields := []interface{}{apmzap.FieldKeyTraceID, traceCtx.Trace, apmzap.FieldKeyTransactionID, traceCtx.Span}

	if span := apm.SpanFromContext(ctx); span != nil {
		fields = append(fields, apmzap.FieldKeySpanID, span.TraceContext().Span)
	}

	return fields
}

type ctxKey struct{}

var loggerCtxKey = ctxKey{}

// InitInContext initializes a logger named `loggerName` with `keysAndValues` and transaction metadata values.
// Returns a context containing the newly created logger.
func InitInContext(ctx context.Context, loggerName string, keysAndValues ...interface{}) context.Context {
	logger := NewFromContext(ctx).WithName(loggerName).WithValues(keysAndValues...)
	return AddToContext(ctx, logger)
}

func AddToContext(ctx context.Context, logger logr.Logger) context.Context {
	return context.WithValue(ctx, loggerCtxKey, logger)
}

func FromContext(ctx context.Context) logr.Logger {
	logger := ctx.Value(loggerCtxKey)
	if logger == nil {
		logger = crlog.Log
	}

	result, ok := logger.(logr.Logger)
	if !ok {
		result = crlog.Log
	}

	if fields := TraceContextKV(ctx); len(fields) > 0 {
		result = result.WithValues(fields...)
	}

	return result
}
