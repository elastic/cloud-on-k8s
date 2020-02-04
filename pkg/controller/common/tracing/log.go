// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package tracing

import (
	"fmt"

	"github.com/go-logr/logr"
	pkgerrors "github.com/pkg/errors"
	"go.elastic.co/apm"
)

// NewLogAdapter returns an implementation of the log interface expected by the APM agent.
func NewLogAdapter(log logr.Logger) apm.Logger {
	return &logAdapter{
		log: log,
	}
}

type logAdapter struct {
	log logr.Logger
}

func (l *logAdapter) Errorf(format string, args ...interface{}) {
	l.log.Error(pkgerrors.Errorf(format, args...), "")
}

func (l *logAdapter) Warningf(format string, args ...interface{}) {
	l.log.V(-1).Info(fmt.Sprintf(format, args...))
}

func (l *logAdapter) Debugf(format string, args ...interface{}) {
	l.log.V(1).Info(fmt.Sprintf(format, args...))
}

var _ apm.Logger = &logAdapter{}
var _ apm.WarningLogger = &logAdapter{}
