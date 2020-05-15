// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package log

import (
	"flag"
	"os"
	"strconv"
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	"github.com/elastic/cloud-on-k8s/pkg/dev"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	klog "k8s.io/klog/v2"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	EcsVersion     = "1.4.0"
	EcsServiceType = "eck"
)

var verbosity = flag.Int("log-verbosity", 0, "Verbosity level of logs (-2=Error, -1=Warn, 0=Info, >0=Debug)")

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
//  1    | -1        | Debug
//  0    |  0        | Info
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

	opts := []zap.Option{zap.Fields(
		zap.String("service.version", getVersionString()),
	)}

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
	var version strings.Builder
	buildInfo := about.GetBuildInfo()

	version.WriteString(buildInfo.Version)
	version.WriteString("-")
	version.WriteString(buildInfo.Hash)

	return version.String()
}
