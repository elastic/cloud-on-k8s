// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package log

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	"github.com/elastic/cloud-on-k8s/pkg/dev"
	"github.com/go-logr/zapr"
	pflag "github.com/spf13/pflag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/klog"
	crlog "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	verbosity       = flag.Int("log-verbosity", 0, "Verbosity level of logs (-2=Error, -1=Warn, 0=Info, >0=Debug)")
	enableDebugLogs = flag.Bool("enable-debug-logs", false, "Enable debug logs")
)

// BindFlags attaches logging flags to the given flag set.
func BindFlags(flags *pflag.FlagSet) {
	flags.AddGoFlag(flag.Lookup("log-verbosity"))
	flags.AddGoFlag(flag.Lookup("enable-debug-logs"))
}

// InitLogger initializes the global logger informed by the values of log-verbosity and enable-debug-logs flags.
func InitLogger() {
	setLogger(verbosity, enableDebugLogs)
}

// ChangeVerbosity replaces the global logger with a new logger set to the sepcified verbosity level.
// Verbosity levels from 2 are custom levels that increase the verbosity as the value increases.
// Standard levels are as follows:
// level | Zap level | name
// -------------------------
//  1    | -1        | Debug
//  0    |  0        | Info
// -1    |  1        | Warn
// -2    |  2        | Error
func ChangeVerbosity(v int) {
	debugLogs := false
	setLogger(&v, &debugLogs)
}

func setLogger(v *int, debug *bool) {
	level := determineLogLevel(v, debug)

	// if the level is higher than 1 set the klog level to the same level
	if level.Level() < zap.DebugLevel {
		flagset := flag.NewFlagSet("", flag.ContinueOnError)
		klog.InitFlags(flagset)
		_ = flagset.Set("v", strconv.Itoa(int(level.Level())*-1))
	}

	var encoder zapcore.Encoder
	var opts []zap.Option
	if dev.Enabled {
		encoderConf := zap.NewDevelopmentEncoderConfig()
		encoderConf.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConf)

		opts = append(opts, zap.Development(), zap.AddStacktrace(zap.ErrorLevel))
	} else {
		encoderConf := zap.NewProductionEncoderConfig()
		encoderConf.MessageKey = "message"
		encoderConf.TimeKey = "@timestamp"
		encoderConf.EncodeTime = zapcore.ISO8601TimeEncoder
		encoder = zapcore.NewJSONEncoder(encoderConf)

		opts = append(opts, zap.AddStacktrace(zap.WarnLevel))
		if level.Level() > zap.DebugLevel {
			opts = append(opts, zap.WrapCore(func(core zapcore.Core) zapcore.Core {
				return zapcore.NewSampler(core, time.Second, 100, 100)
			}))
		}
	}

	sink := zapcore.AddSync(os.Stderr)
	opts = append(opts, zap.AddCallerSkip(1), zap.ErrorOutput(sink))
	log := zap.New(zapcore.NewCore(&crlog.KubeAwareEncoder{Encoder: encoder, Verbose: dev.Enabled}, sink, level))
	log = log.WithOptions(opts...)
	log = log.With(zap.String("ver", getVersionString()))

	crlog.SetLogger(zapr.NewLogger(log))
}

func determineLogLevel(v *int, debug *bool) zap.AtomicLevel {
	switch {
	case v != nil && *v > -3:
		return zap.NewAtomicLevelAt(zapcore.Level(*v * -1))
	case debug != nil && *debug:
		return zap.NewAtomicLevelAt(zapcore.DebugLevel)
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
