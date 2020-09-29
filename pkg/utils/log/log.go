// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package log

import (
	"flag"
	"os"
	"strconv"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	"github.com/elastic/cloud-on-k8s/pkg/dev"
	"github.com/spf13/pflag"
	"go.elastic.co/apm"
	"go.elastic.co/apm/module/apmzap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	klog "k8s.io/klog/v2"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	EcsVersion     = "1.4.0"
	EcsServiceType = "eck"
	FlagName       = "log-verbosity"
)

var verbosity = flag.Int(FlagName, 0, "Verbosity level of logs (-2=Error, -1=Warn, 0=Info, >0=Debug)")

// BindFlags attaches logging flags to the given flag set.
func BindFlags(flags *pflag.FlagSet) {
	flags.AddGoFlag(flag.Lookup("log-verbosity"))
}

type logBuilder struct {
	tracer    *apm.Tracer
	verbosity *int
}

// Option represents log configuration options.
type Option func(*logBuilder)

// WithVerbosity sets the log verbosity level.
// Verbosity levels from 2 are custom levels that increase the verbosity as the value increases.
// Standard levels are as follows:
// level | Zap level | name
// -------------------------
//  1    | -1        | Debug
//  0    |  0        | Info
// -1    |  1        | Warn
// -2    |  2        | Error
func WithVerbosity(verbosity int) Option {
	return func(lb *logBuilder) {
		lb.verbosity = &verbosity
	}
}

// WithTracer sets the tracer used by the logger to send logs to APM.
func WithTracer(tracer *apm.Tracer) Option {
	return func(lb *logBuilder) {
		lb.tracer = tracer
	}
}

// InitLogger initializes the global logger.
func InitLogger(opts ...Option) {
	lb := &logBuilder{
		verbosity: verbosity,
	}

	for _, opt := range opts {
		opt(lb)
	}

	setLogger(lb.verbosity, lb.tracer)
}

func setLogger(v *int, tracer *apm.Tracer) {
	zapLevel := determineLogLevel(v)

	// if the Zap custom level is less than debug (verbosity level 2 and above) set the klog level to the same level
	if zapLevel.Level() < zap.DebugLevel {
		flagset := flag.NewFlagSet("", flag.ContinueOnError)
		klog.InitFlags(flagset)
		_ = flagset.Set("v", strconv.Itoa(int(zapLevel.Level())*-1))
	}

	opts := []zap.Option{
		zap.Fields(
			zap.String("service.version", getVersionString()),
		),
	}

	// use instrumented core if tracing is enabled
	if tracer != nil {
		opts = append(opts, zap.WrapCore((&apmzap.Core{Tracer: tracer}).WrapCore))
	}

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
		o.DestWritter = os.Stderr
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
