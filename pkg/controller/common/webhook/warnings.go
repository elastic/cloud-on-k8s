// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"k8s.io/apimachinery/pkg/runtime"
)

type HasWarnings interface {
	GetWarnings() []string
}

func MaybeGetWarnings(object runtime.Object) []string {
	v, ok := object.(HasWarnings)
	if ok {
		return v.GetWarnings()
	}
	return nil
}
