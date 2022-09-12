// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/ghodss/yaml"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/elastic/cloud-on-k8s/v2/hack/upgrade-test-harness/fixture"
)

// File represents the structure of the configuration file.
type File struct {
	TestParams []fixture.TestParam `json:"testParams"`
}

// ReleasePos returns the position of the given release in the ordered list.
func (f *File) ReleasePos(name string) (int, error) {
	for i, release := range f.TestParams {
		if name == release.Name {
			return i, nil
		}
	}

	return -1, fmt.Errorf("unable to find release named %s", name)
}

// Load the configuration from a YAML file.
func Load(path string) (*File, error) {
	confBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var conf File
	if err := yaml.Unmarshal(confBytes, &conf); err != nil {
		return nil, err
	}

	return &conf, nil
}

// InitLogging initialized logging with Zap.
func InitLogging(level string) {
	var logger *zap.Logger

	errorPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.ErrorLevel
	})

	minLogLevel := zapcore.InfoLevel

	switch strings.ToUpper(level) {
	case "DEBUG":
		minLogLevel = zapcore.DebugLevel
	case "INFO":
		minLogLevel = zapcore.InfoLevel
	case "WARN":
		minLogLevel = zapcore.WarnLevel
	case "ERROR":
		minLogLevel = zapcore.ErrorLevel
	}

	infoPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl < zapcore.ErrorLevel && lvl >= minLogLevel
	})

	consoleErrors := zapcore.Lock(os.Stderr)
	consoleInfo := zapcore.Lock(os.Stdout)

	encoderConf := zap.NewDevelopmentEncoderConfig()
	encoderConf.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleEncoder := zapcore.NewConsoleEncoder(encoderConf)

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, consoleErrors, errorPriority),
		zapcore.NewCore(consoleEncoder, consoleInfo, infoPriority),
	)

	stackTraceEnabler := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl > zapcore.ErrorLevel
	})
	logger = zap.New(core, zap.AddStacktrace(stackTraceEnabler))

	zap.ReplaceGlobals(logger.Named("eck-upgrade"))
	zap.RedirectStdLog(logger.Named("stdlog"))
}
