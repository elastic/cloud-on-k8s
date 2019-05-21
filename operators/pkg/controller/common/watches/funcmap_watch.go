// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package watches

import (
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

type FuncMapWatch struct {
	// Name identifies this watch for easier removal and deduplication.
	Name string

	handler.EnqueueRequestsFromMapFunc
}

func (f *FuncMapWatch) Key() string {
	return f.Name
}

func (f *FuncMapWatch) EventHandler() handler.EventHandler {
	return f
}

var _ HandlerRegistration = &FuncMapWatch{}
