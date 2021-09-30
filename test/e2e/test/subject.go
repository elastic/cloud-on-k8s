// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Subject the main test subject for a test case
type Subject interface {
	NSN() types.NamespacedName
	Kind() string
	Spec() interface{}
	Count() int32
	ServiceName() string
	ListOptions() []k8sclient.ListOption
}
