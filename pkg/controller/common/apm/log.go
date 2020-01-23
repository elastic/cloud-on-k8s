// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apm

import (
	"fmt"

	"github.com/go-logr/logr"
	"go.elastic.co/apm"
)

func NewLogAdapter(log logr.Logger) apm.Logger {
	return &logAdapter{
		log: log,
	}
}

type logAdapter struct {
	log logr.Logger
}

func (l *logAdapter) Debugf(format string, args ...interface{}) {
	l.log.Info(fmt.Sprintf(format, args...))
}

func (l *logAdapter) Errorf(format string, args ...interface{}) {
	l.log.Error(fmt.Errorf(format, args...), "")
}

var _ apm.Logger = &logAdapter{}
